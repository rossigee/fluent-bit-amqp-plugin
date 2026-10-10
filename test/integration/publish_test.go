//go:build integration

package integration

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	cesdk "github.com/cloudevents/sdk-go/v2"
	amqplib "github.com/rabbitmq/amqp091-go"
	"github.com/rossigee/fluent-bit-amqp-plugin/pkg/amqp"
	"github.com/rossigee/fluent-bit-amqp-plugin/pkg/cloudevents"
	"github.com/rossigee/fluent-bit-amqp-plugin/pkg/config"
)

func rabbitmqURL(t *testing.T) string {
	t.Helper()
	url := os.Getenv("RABBITMQ_URL")
	if url == "" {
		url = "amqp://guest:guest@localhost:5672/"
	}
	// Skip rather than fail when the broker is not reachable.
	conn, err := amqplib.Dial(url)
	if err != nil {
		t.Skipf("RabbitMQ not available at %s (set RABBITMQ_URL to override): %v", url, err)
	}
	conn.Close() //nolint:errcheck
	return url
}

// newTestPublisher creates a publisher connected to the test RabbitMQ, declaring
// a unique transient queue for the test and returning the queue name.
func newTestPublisher(t *testing.T, queueName string) (*amqp.Publisher, func()) {
	t.Helper()
	cfg := config.DefaultConfig()
	cfg.URL = rabbitmqURL(t)
	cfg.Queue = queueName
	cfg.RoutingKey = queueName
	// RabbitMQ 4.x rejects non-exclusive transient queues, so test queues are durable
	// and removed by the returned cleanup function.
	cfg.Durable = true
	cfg.AutoDelete = false

	pub, err := amqp.NewPublisher(cfg)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	return pub, func() {
		if err := pub.Close(); err != nil {
			t.Logf("Close: %v", err)
		}
		deleteQueue(t, cfg.URL, queueName)
	}
}

// consumeTimeout bounds how long consume waits for messages to become visible.
const consumeTimeout = 5 * time.Second

// consume drains up to n messages from queueName using a direct AMQP connection.
func consume(t *testing.T, url, queueName string, n int) []amqplib.Delivery {
	t.Helper()
	conn, err := amqplib.Dial(url)
	if err != nil {
		t.Fatalf("consume dial: %v", err)
	}
	defer conn.Close() //nolint:errcheck

	ch, err := conn.Channel()
	if err != nil {
		t.Fatalf("consume channel: %v", err)
	}
	defer ch.Close() //nolint:errcheck

	var msgs []amqplib.Delivery
	deadline := time.Now().Add(consumeTimeout)
	for len(msgs) < n {
		msg, ok, err := ch.Get(queueName, true)
		if err != nil {
			t.Fatalf("ch.Get: %v", err)
		}
		if ok {
			msgs = append(msgs, msg)
			continue
		}
		// Messages published without publisher confirms may not be visible to
		// basic.get immediately, so poll until the deadline.
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	return msgs
}

// TestPublishCloudEvents verifies that CloudEvents-wrapped messages are published
// correctly and can be consumed and decoded from RabbitMQ.
func TestPublishCloudEvents(t *testing.T) {
	queueName := "integration-test-cloudevents"
	pub, cleanup := newTestPublisher(t, queueName)
	defer cleanup()

	wrapper := cloudevents.NewWrapper(&cloudevents.WrapperConfig{
		Source: "integration-test",
		Type:   "test.event",
	})

	ts := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	record := map[string]interface{}{
		"message": "hello from integration test",
		"level":   "info",
	}

	event, err := wrapper.WrapRecord(ts, record, "test.tag")
	if err != nil {
		t.Fatalf("WrapRecord: %v", err)
	}

	if err := pub.PublishCloudEvent(event); err != nil {
		t.Fatalf("PublishCloudEvent: %v", err)
	}

	msgs := consume(t, rabbitmqURL(t), queueName, 1)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}

	msg := msgs[0]

	// Content type
	if msg.ContentType != "application/json" {
		t.Errorf("ContentType: got %q, want %q", msg.ContentType, "application/json")
	}

	// CE headers
	for _, header := range []string{"ce-specversion", "ce-type", "ce-source", "ce-id"} {
		if _, ok := msg.Headers[header]; !ok {
			t.Errorf("missing AMQP header %q", header)
		}
	}
	if got, _ := msg.Headers["ce-source"].(string); got != "integration-test" {
		t.Errorf("ce-source: got %q, want %q", got, "integration-test")
	}
	if got, _ := msg.Headers["ce-type"].(string); got != "test.event" {
		t.Errorf("ce-type: got %q, want %q", got, "test.event")
	}

	// Body must decode as a CloudEvent with the right data
	var decoded cesdk.Event
	if err := json.Unmarshal(msg.Body, &decoded); err != nil {
		t.Fatalf("body not a CloudEvent: %v\nbody: %s", err, msg.Body)
	}
	if decoded.Source() != "integration-test" {
		t.Errorf("event source: got %q, want %q", decoded.Source(), "integration-test")
	}
	if decoded.Type() != "test.event" {
		t.Errorf("event type: got %q, want %q", decoded.Type(), "test.event")
	}

	var data map[string]interface{}
	if err := json.Unmarshal(decoded.Data(), &data); err != nil {
		t.Fatalf("event data not JSON: %v", err)
	}
	if data["message"] != "hello from integration test" {
		t.Errorf("data.message: got %v", data["message"])
	}
}

// TestPublishCloudEvents_NestedRecord verifies that nested map[interface{}]interface{}
// records (as produced by Lua filters in Fluent Bit) are normalised and published
// without error — this is the root cause documented in issue #6.
func TestPublishCloudEvents_NestedRecord(t *testing.T) {
	queueName := "integration-test-cloudevents-nested"
	pub, cleanup := newTestPublisher(t, queueName)
	defer cleanup()

	wrapper := cloudevents.NewWrapper(&cloudevents.WrapperConfig{
		Source: "integration-test",
		Type:   "test.event",
	})

	ts := time.Now()
	// Simulate a Lua record with nested map[interface{}]interface{} types
	record := map[interface{}]interface{}{
		"message": "alert triggered",
		"level":   "warn",
		"custom_fields": map[interface{}]interface{}{
			"host":       "server-01",
			"datacenter": "eu-west-1",
		},
	}

	event, err := wrapper.WrapRecord(ts, record, "alert.rabbitmq")
	if err != nil {
		t.Fatalf("WrapRecord with nested map: %v", err)
	}

	if err := pub.PublishCloudEvent(event); err != nil {
		t.Fatalf("PublishCloudEvent with nested map: %v", err)
	}

	msgs := consume(t, rabbitmqURL(t), queueName, 1)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}

	var decoded cesdk.Event
	if err := json.Unmarshal(msgs[0].Body, &decoded); err != nil {
		t.Fatalf("body not a CloudEvent: %v", err)
	}

	var data map[string]interface{}
	if err := json.Unmarshal(decoded.Data(), &data); err != nil {
		t.Fatalf("event data not JSON: %v", err)
	}
	if data["message"] != "alert triggered" {
		t.Errorf("data.message: got %v", data["message"])
	}
	cf, ok := data["custom_fields"].(map[string]interface{})
	if !ok {
		t.Fatalf("custom_fields not a map[string]interface{}: %T", data["custom_fields"])
	}
	if cf["host"] != "server-01" {
		t.Errorf("custom_fields.host: got %v", cf["host"])
	}
}

// TestPublishRecord_PlainJSON verifies that plain JSON messages are published
// correctly and contain the expected @timestamp and tag fields.
func TestPublishRecord_PlainJSON(t *testing.T) {
	queueName := "integration-test-plain"
	pub, cleanup := newTestPublisher(t, queueName)
	defer cleanup()

	ts := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	record := map[string]interface{}{
		"message": "plain json test",
		"level":   "debug",
	}

	if err := pub.PublishRecord(ts, record, "test.plain"); err != nil {
		t.Fatalf("PublishRecord: %v", err)
	}

	msgs := consume(t, rabbitmqURL(t), queueName, 1)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}

	msg := msgs[0]

	if msg.ContentType != "application/json" {
		t.Errorf("ContentType: got %q, want %q", msg.ContentType, "application/json")
	}

	// No CE headers
	for _, header := range []string{"ce-specversion", "ce-type", "ce-source", "ce-id"} {
		if _, ok := msg.Headers[header]; ok {
			t.Errorf("unexpected AMQP header %q in plain JSON message", header)
		}
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(msg.Body, &payload); err != nil {
		t.Fatalf("body not JSON: %v\nbody: %s", err, msg.Body)
	}

	if payload["@timestamp"] != "2024-06-01T12:00:00Z" {
		t.Errorf("@timestamp: got %v", payload["@timestamp"])
	}
	if payload["tag"] != "test.plain" {
		t.Errorf("tag: got %v", payload["tag"])
	}
	if payload["message"] != "plain json test" {
		t.Errorf("message: got %v", payload["message"])
	}
	if payload["level"] != "debug" {
		t.Errorf("level: got %v", payload["level"])
	}
}

// TestPublishRecord_PlainJSON_NestedRecord verifies that nested Lua records
// are normalised correctly in plain JSON mode.
func TestPublishRecord_PlainJSON_NestedRecord(t *testing.T) {
	queueName := "integration-test-plain-nested"
	pub, cleanup := newTestPublisher(t, queueName)
	defer cleanup()

	ts := time.Now()
	record := map[interface{}]interface{}{
		"message": "nested plain test",
		"context": map[interface{}]interface{}{
			"service": "auth",
			"region":  "us-east-1",
		},
	}

	if err := pub.PublishRecord(ts, record, "test.nested"); err != nil {
		t.Fatalf("PublishRecord with nested map: %v", err)
	}

	msgs := consume(t, rabbitmqURL(t), queueName, 1)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(msgs[0].Body, &payload); err != nil {
		t.Fatalf("body not JSON: %v", err)
	}

	ctx, ok := payload["context"].(map[string]interface{})
	if !ok {
		t.Fatalf("context not map[string]interface{}: %T", payload["context"])
	}
	if ctx["service"] != "auth" {
		t.Errorf("context.service: got %v", ctx["service"])
	}
}

// TestPublishRecord_NoTag verifies that an empty tag is omitted from the payload.
func TestPublishRecord_NoTag(t *testing.T) {
	queueName := "integration-test-plain-notag"
	pub, cleanup := newTestPublisher(t, queueName)
	defer cleanup()

	ts := time.Now()
	record := map[string]interface{}{"msg": "no tag"}

	if err := pub.PublishRecord(ts, record, ""); err != nil {
		t.Fatalf("PublishRecord: %v", err)
	}

	msgs := consume(t, rabbitmqURL(t), queueName, 1)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(msgs[0].Body, &payload); err != nil {
		t.Fatalf("body not JSON: %v", err)
	}

	if _, ok := payload["tag"]; ok {
		t.Error("expected tag field to be absent when tag is empty")
	}
}

// tlsClientConfig builds the consumer-side TLS config from certificate files
// supplied via the environment. Empty paths are skipped.
func tlsClientConfig(t *testing.T, caFile, certFile, keyFile string) *tls.Config {
	t.Helper()
	cfg := &tls.Config{}
	if caFile != "" {
		caPEM, err := os.ReadFile(caFile)
		if err != nil {
			t.Fatalf("reading CA file: %v", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caPEM) {
			t.Fatalf("parsing CA file %s: no certificates found", caFile)
		}
		cfg.RootCAs = pool
	}
	if certFile != "" && keyFile != "" {
		cert, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			t.Fatalf("loading client keypair: %v", err)
		}
		cfg.Certificates = []tls.Certificate{cert}
	}
	return cfg
}

// TestPublishTLS verifies that records are published over AMQPS with server
// certificate verification and, when configured, client certificate
// authentication. It requires a TLS-enabled broker and is skipped unless
// RABBITMQ_TLS_URL is set. Certificates are supplied from existing files:
//
//	RABBITMQ_TLS_URL        amqps:// URL of the broker
//	RABBITMQ_TLS_CA_FILE    CA certificate that signed the broker certificate (optional; system roots otherwise)
//	RABBITMQ_TLS_CERT_FILE  client certificate (optional)
//	RABBITMQ_TLS_KEY_FILE   client private key (optional)
func TestPublishTLS(t *testing.T) {
	url := os.Getenv("RABBITMQ_TLS_URL")
	if url == "" {
		t.Skip("RABBITMQ_TLS_URL not set; skipping TLS integration test")
	}
	caFile := os.Getenv("RABBITMQ_TLS_CA_FILE")
	certFile := os.Getenv("RABBITMQ_TLS_CERT_FILE")
	keyFile := os.Getenv("RABBITMQ_TLS_KEY_FILE")

	queueName := "integration-test-tls"
	cfg := config.DefaultConfig()
	cfg.URL = url
	cfg.TLS = true
	cfg.TLSCAFile = caFile
	cfg.TLSCertFile = certFile
	cfg.TLSKeyFile = keyFile
	cfg.Queue = queueName
	cfg.RoutingKey = queueName
	cfg.Durable = false
	cfg.AutoDelete = true

	pub, err := amqp.NewPublisher(cfg)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer func() {
		if err := pub.Close(); err != nil {
			t.Logf("Close: %v", err)
		}
	}()

	ts := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	record := map[string]interface{}{"message": "hello over tls"}
	if err := pub.PublishRecord(ts, record, "test.tls"); err != nil {
		t.Fatalf("PublishRecord over TLS: %v", err)
	}

	conn, err := amqplib.DialConfig(url, amqplib.Config{
		TLSClientConfig: tlsClientConfig(t, caFile, certFile, keyFile),
	})
	if err != nil {
		t.Fatalf("consumer dial over TLS: %v", err)
	}
	defer conn.Close() //nolint:errcheck

	ch, err := conn.Channel()
	if err != nil {
		t.Fatalf("consumer channel: %v", err)
	}
	defer ch.Close() //nolint:errcheck

	msg, ok, err := ch.Get(queueName, true)
	if err != nil {
		t.Fatalf("ch.Get: %v", err)
	}
	if !ok {
		t.Fatal("expected 1 message over TLS, got none")
	}
	if !strings.Contains(string(msg.Body), "hello over tls") {
		t.Errorf("unexpected body: %s", msg.Body)
	}
}

// deleteQueue removes a test queue so repeated runs start clean.
func deleteQueue(t *testing.T, url, queueName string) {
	t.Helper()
	conn, err := amqplib.Dial(url)
	if err != nil {
		t.Logf("cleanup dial: %v", err)
		return
	}
	defer conn.Close() //nolint:errcheck

	ch, err := conn.Channel()
	if err != nil {
		t.Logf("cleanup channel: %v", err)
		return
	}
	defer ch.Close() //nolint:errcheck

	if _, err := ch.QueueDelete(queueName, false, false, false); err != nil {
		t.Logf("cleanup delete %s: %v", queueName, err)
	}
}

// mgmtQueue returns the queue's details from the RabbitMQ management API, or nil
// when RABBITMQ_MGMT_URL (for example http://localhost:15672) is not set. Credentials
// and vhost are taken from the AMQP URL.
func mgmtQueue(t *testing.T, amqpURL, queueName string) map[string]interface{} {
	t.Helper()
	base := os.Getenv("RABBITMQ_MGMT_URL")
	if base == "" {
		return nil
	}
	u, err := url.Parse(amqpURL)
	if err != nil {
		t.Fatalf("parsing AMQP URL: %v", err)
	}
	vhost := strings.TrimPrefix(u.Path, "/")
	if vhost == "" {
		vhost = "/"
	}
	password, _ := u.User.Password()

	endpoint := strings.TrimRight(base, "/") + "/api/queues/" + url.PathEscape(vhost) + "/" + url.PathEscape(queueName)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, endpoint, nil)
	if err != nil {
		t.Fatalf("building management request: %v", err)
	}
	req.SetBasicAuth(u.User.Username(), password)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("management API request: %v", err)
	}
	defer resp.Body.Close() //nolint:errcheck
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("management API %s: status %d", endpoint, resp.StatusCode)
	}

	var info map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		t.Fatalf("decoding management API response: %v", err)
	}
	return info
}

// newQueueConfig returns a config for a uniquely named queue that is cleaned up by the caller.
func newQueueConfig(url, queueName string) *config.AMQPConfig {
	cfg := config.DefaultConfig()
	cfg.URL = url
	cfg.Queue = queueName
	cfg.RoutingKey = queueName
	// Durable so RabbitMQ 4.x accepts the declaration; callers remove the queue.
	cfg.Durable = true
	cfg.AutoDelete = false
	return cfg
}

// TestQueueTypeQuorum verifies that queue_type=quorum declares a quorum queue and
// that records published to it can be consumed.
func TestQueueTypeQuorum(t *testing.T) {
	url := rabbitmqURL(t)
	queueName := "integration-test-quorum"
	defer deleteQueue(t, url, queueName)

	cfg := newQueueConfig(url, queueName)
	cfg.QueueType = config.QueueTypeQuorum
	cfg.Durable = true
	cfg.AutoDelete = false

	pub, err := amqp.NewPublisher(cfg)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close() //nolint:errcheck

	if err := pub.PublishRecord(time.Now(), map[string]interface{}{"message": "quorum"}, "test.quorum"); err != nil {
		t.Fatalf("PublishRecord to quorum queue: %v", err)
	}
	if msgs := consume(t, url, queueName, 1); len(msgs) != 1 {
		t.Fatalf("expected 1 message from quorum queue, got %d", len(msgs))
	}

	if info := mgmtQueue(t, url, queueName); info == nil {
		t.Log("RABBITMQ_MGMT_URL not set; queue type not verified")
	} else if info["type"] != "quorum" {
		t.Errorf("queue type: got %v, want quorum", info["type"])
	}
}

// TestQueueTypeStream verifies that queue_type=stream declares a stream queue and that
// publishing to it succeeds. Streams cannot be read with basic.get, so only the declared
// type is checked.
func TestQueueTypeStream(t *testing.T) {
	url := rabbitmqURL(t)
	queueName := "integration-test-stream"
	defer deleteQueue(t, url, queueName)

	cfg := newQueueConfig(url, queueName)
	cfg.QueueType = config.QueueTypeStream
	cfg.Durable = true
	cfg.AutoDelete = false

	pub, err := amqp.NewPublisher(cfg)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close() //nolint:errcheck

	if err := pub.PublishRecord(time.Now(), map[string]interface{}{"message": "stream"}, "test.stream"); err != nil {
		t.Fatalf("PublishRecord to stream queue: %v", err)
	}

	if info := mgmtQueue(t, url, queueName); info == nil {
		t.Log("RABBITMQ_MGMT_URL not set; queue type not verified")
	} else if info["type"] != "stream" {
		t.Errorf("queue type: got %v, want stream", info["type"])
	}
}

// TestQueueModeLazy verifies that queue_mode=lazy declares a classic queue with x-queue-mode=lazy.
func TestQueueModeLazy(t *testing.T) {
	url := rabbitmqURL(t)
	queueName := "integration-test-lazy"
	defer deleteQueue(t, url, queueName)

	cfg := newQueueConfig(url, queueName)
	cfg.QueueMode = config.QueueModeLazy

	pub, err := amqp.NewPublisher(cfg)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close() //nolint:errcheck

	if err := pub.PublishRecord(time.Now(), map[string]interface{}{"message": "lazy"}, "test.lazy"); err != nil {
		t.Fatalf("PublishRecord to lazy queue: %v", err)
	}

	if info := mgmtQueue(t, url, queueName); info == nil {
		t.Log("RABBITMQ_MGMT_URL not set; queue mode not verified")
	} else {
		args, _ := info["arguments"].(map[string]interface{})
		if args["x-queue-mode"] != "lazy" {
			t.Errorf("x-queue-mode: got %v, want lazy", args["x-queue-mode"])
		}
	}
}

// TestMessagePropertiesPublished verifies that persistent, priority, message type,
// app id and content type reach the consumer as AMQP message properties.
func TestMessagePropertiesPublished(t *testing.T) {
	url := rabbitmqURL(t)
	queueName := "integration-test-properties"
	defer deleteQueue(t, url, queueName)

	cfg := newQueueConfig(url, queueName)
	cfg.Persistent = true
	cfg.Priority = 5
	cfg.MessageType = "log"
	cfg.AppID = "edge-01"

	pub, err := amqp.NewPublisher(cfg)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close() //nolint:errcheck

	if err := pub.PublishRecord(time.Now(), map[string]interface{}{"message": "props"}, "test.props"); err != nil {
		t.Fatalf("PublishRecord: %v", err)
	}

	msgs := consume(t, url, queueName, 1)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	msg := msgs[0]

	if msg.DeliveryMode != amqplib.Persistent {
		t.Errorf("DeliveryMode: got %d, want %d (persistent)", msg.DeliveryMode, amqplib.Persistent)
	}
	if msg.Priority != 5 {
		t.Errorf("Priority: got %d, want 5", msg.Priority)
	}
	if msg.Type != "log" {
		t.Errorf("Type: got %q, want log", msg.Type)
	}
	if msg.AppId != "edge-01" {
		t.Errorf("AppId: got %q, want edge-01", msg.AppId)
	}
	if msg.ContentType != "application/json" {
		t.Errorf("ContentType: got %q, want application/json", msg.ContentType)
	}
}

// TestExpiredMessageIsRemoved verifies that a message published with expiration is
// not delivered after its TTL has elapsed.
func TestExpiredMessageIsRemoved(t *testing.T) {
	url := rabbitmqURL(t)
	queueName := "integration-test-expiration"
	defer deleteQueue(t, url, queueName)

	cfg := newQueueConfig(url, queueName)
	cfg.Expiration = 500

	pub, err := amqp.NewPublisher(cfg)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close() //nolint:errcheck

	if err := pub.PublishRecord(time.Now(), map[string]interface{}{"message": "expires"}, "test.expiry"); err != nil {
		t.Fatalf("PublishRecord: %v", err)
	}

	time.Sleep(1500 * time.Millisecond)

	if msgs := consume(t, url, queueName, 1); len(msgs) != 0 {
		t.Errorf("expected expired message to be removed, got %d message(s)", len(msgs))
	}
}

// waitForBroker polls until the broker accepts AMQP connections or the timeout elapses.
func waitForBroker(t *testing.T, url string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		conn, err := amqplib.Dial(url)
		if err == nil {
			conn.Close() //nolint:errcheck
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("broker not ready after %s: %v", timeout, err)
		}
		time.Sleep(time.Second)
	}
}

// TestPersistentMessageSurvivesRestart publishes a persistent message to a durable
// quorum queue, restarts the broker container and checks the message is still there.
// It requires RABBITMQ_TEST_CONTAINER to name the broker's Docker container and is
// skipped otherwise.
func TestPersistentMessageSurvivesRestart(t *testing.T) {
	container := os.Getenv("RABBITMQ_TEST_CONTAINER")
	if container == "" {
		t.Skip("RABBITMQ_TEST_CONTAINER not set; skipping broker restart test")
	}
	url := rabbitmqURL(t)
	queueName := "integration-test-persistent-restart"
	defer deleteQueue(t, url, queueName)

	cfg := newQueueConfig(url, queueName)
	cfg.QueueType = config.QueueTypeQuorum
	cfg.Durable = true
	cfg.AutoDelete = false
	cfg.Persistent = true

	pub, err := amqp.NewPublisher(cfg)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	if err := pub.PublishRecord(time.Now(), map[string]interface{}{"message": "survives restart"}, "test.restart"); err != nil {
		t.Fatalf("PublishRecord: %v", err)
	}
	if err := pub.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	out, err := exec.Command("docker", "restart", container).CombinedOutput()
	if err != nil {
		t.Fatalf("docker restart %s: %v: %s", container, err, out)
	}
	waitForBroker(t, url, 90*time.Second)

	msgs := consume(t, url, queueName, 1)
	if len(msgs) != 1 {
		t.Fatalf("expected persistent message after restart, got %d", len(msgs))
	}
	if !strings.Contains(string(msgs[0].Body), "survives restart") {
		t.Errorf("unexpected body after restart: %s", msgs[0].Body)
	}
}

// TestPublisherReconnectsAfterBrokerRestart keeps a publisher open across a broker
// restart and checks that the next publish re-establishes the connection and succeeds.
// It requires RABBITMQ_TEST_CONTAINER and is skipped otherwise.
func TestPublisherReconnectsAfterBrokerRestart(t *testing.T) {
	container := os.Getenv("RABBITMQ_TEST_CONTAINER")
	if container == "" {
		t.Skip("RABBITMQ_TEST_CONTAINER not set; skipping reconnect test")
	}
	url := rabbitmqURL(t)
	queueName := "integration-test-reconnect"
	defer deleteQueue(t, url, queueName)

	cfg := newQueueConfig(url, queueName)
	cfg.QueueType = config.QueueTypeQuorum
	cfg.AutoDelete = false

	pub, err := amqp.NewPublisher(cfg)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close() //nolint:errcheck

	if err := pub.PublishRecord(time.Now(), map[string]interface{}{"message": "before restart"}, "test.reconnect"); err != nil {
		t.Fatalf("PublishRecord before restart: %v", err)
	}

	out, err := exec.Command("docker", "restart", container).CombinedOutput()
	if err != nil {
		t.Fatalf("docker restart %s: %v: %s", container, err, out)
	}
	waitForBroker(t, url, 90*time.Second)

	if err := pub.PublishRecord(time.Now(), map[string]interface{}{"message": "after restart"}, "test.reconnect"); err != nil {
		t.Fatalf("PublishRecord after restart (expected reconnect): %v", err)
	}

	msgs := consume(t, url, queueName, 2)
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages across restart, got %d", len(msgs))
	}
}
