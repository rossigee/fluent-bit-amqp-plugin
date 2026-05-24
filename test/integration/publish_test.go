//go:build integration

package integration

import (
	"encoding/json"
	"os"
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
	cfg.Durable = false
	cfg.AutoDelete = true

	pub, err := amqp.NewPublisher(cfg)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	return pub, func() {
		if err := pub.Close(); err != nil {
			t.Logf("Close: %v", err)
		}
	}
}

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
	for i := 0; i < n; i++ {
		msg, ok, err := ch.Get(queueName, true)
		if err != nil {
			t.Fatalf("ch.Get: %v", err)
		}
		if !ok {
			break
		}
		msgs = append(msgs, msg)
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
