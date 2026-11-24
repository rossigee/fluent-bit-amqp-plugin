//go:build integration

package integration

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"
	"github.com/rossigee/fluent-bit-amqp-plugin/pkg/cloudevents"
	"github.com/rossigee/fluent-bit-amqp-plugin/pkg/config"
	mqttpub "github.com/rossigee/fluent-bit-amqp-plugin/pkg/mqtt"
)

// mqttBroker returns the MQTT broker URL, or skips the test when none is configured.
func mqttBroker(t *testing.T) string {
	t.Helper()
	broker := os.Getenv("MQTT_URL")
	if broker == "" {
		t.Skip("MQTT_URL not set; skipping MQTT integration test")
	}
	return broker
}

// mqttSubscribe subscribes to topic and returns a channel of received payloads.
func mqttSubscribe(t *testing.T, broker, topic string, qos byte) <-chan []byte {
	t.Helper()
	received := make(chan []byte, 16)
	opts := paho.NewClientOptions().
		AddBroker(broker).
		SetClientID(fmt.Sprintf("it-sub-%d", time.Now().UnixNano())).
		SetConnectTimeout(10 * time.Second)
	client := paho.NewClient(opts)
	if token := client.Connect(); !token.WaitTimeout(10*time.Second) || token.Error() != nil {
		t.Fatalf("subscriber connect: %v", token.Error())
	}
	t.Cleanup(func() { client.Disconnect(250) })

	token := client.Subscribe(topic, qos, func(_ paho.Client, msg paho.Message) {
		received <- msg.Payload()
	})
	if !token.WaitTimeout(10*time.Second) || token.Error() != nil {
		t.Fatalf("subscribe %s: %v", topic, token.Error())
	}
	return received
}

// mqttWaitFor returns the next payload on received, failing the test on timeout.
func mqttWaitFor(t *testing.T, received <-chan []byte) []byte {
	t.Helper()
	select {
	case body := <-received:
		return body
	case <-time.After(15 * time.Second):
		t.Fatal("timed out waiting for MQTT message")
		return nil
	}
}

func mqttPublisher(t *testing.T, broker, topic string, qos int) *mqttpub.Publisher {
	t.Helper()
	cfg := config.DefaultConfig()
	cfg.Backend = config.BackendMQTT
	cfg.MQTT.Broker = broker
	cfg.MQTT.Topic = topic
	cfg.MQTT.QoS = qos
	cfg.MQTT.ClientID = fmt.Sprintf("it-pub-%d", time.Now().UnixNano())
	pub, err := mqttpub.NewPublisher(cfg)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	t.Cleanup(func() { _ = pub.Close() }) //nolint:errcheck
	return pub
}

func uniqueTopic(name string) string {
	return fmt.Sprintf("integration/%s/%d", name, time.Now().UnixNano())
}

// TestMQTTPublishRecord verifies that a plain JSON record reaches the topic with its
// fields, timestamp and tag.
func TestMQTTPublishRecord(t *testing.T) {
	broker := mqttBroker(t)
	topic := uniqueTopic("record")
	received := mqttSubscribe(t, broker, topic, 1)

	pub := mqttPublisher(t, broker, topic, 1)
	ts := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	if err := pub.PublishRecord(ts, map[string]interface{}{"message": "mqtt-marker"}, "test.mqtt"); err != nil {
		t.Fatalf("PublishRecord: %v", err)
	}

	var got map[string]interface{}
	if err := json.Unmarshal(mqttWaitFor(t, received), &got); err != nil {
		t.Fatalf("payload is not JSON: %v", err)
	}
	if got["message"] != "mqtt-marker" || got["@timestamp"] != "2024-06-01T12:00:00Z" || got["tag"] != "test.mqtt" {
		t.Errorf("unexpected payload: %v", got)
	}
}

// TestMQTTPublishCloudEvent verifies that a CloudEvent reaches the topic with its
// attributes carried in the JSON body.
func TestMQTTPublishCloudEvent(t *testing.T) {
	broker := mqttBroker(t)
	topic := uniqueTopic("cloudevent")
	received := mqttSubscribe(t, broker, topic, 1)

	pub := mqttPublisher(t, broker, topic, 1)
	wrapper := cloudevents.NewWrapper(&cloudevents.WrapperConfig{Source: "it-source", Type: "it.log"})
	event, err := wrapper.WrapRecord(time.Now(), map[string]interface{}{"message": "ce-marker"}, "test.mqtt")
	if err != nil {
		t.Fatalf("WrapRecord: %v", err)
	}
	if err := pub.PublishCloudEvent(event); err != nil {
		t.Fatalf("PublishCloudEvent: %v", err)
	}

	body := string(mqttWaitFor(t, received))
	for _, want := range []string{`"specversion":"1.0"`, `"type":"it.log"`, `"source":"it-source"`, "ce-marker"} {
		if !strings.Contains(body, want) {
			t.Errorf("CloudEvent body missing %s: %s", want, body)
		}
	}
}

// mqttTLSEnv returns the MQTTS broker URL and certificate files from the environment,
// or skips the test when no TLS broker is configured.
func mqttTLSEnv(t *testing.T) (broker, caFile, certFile, keyFile string) {
	t.Helper()
	broker = os.Getenv("MQTT_TLS_URL")
	if broker == "" {
		t.Skip("MQTT_TLS_URL not set; skipping MQTTS integration test")
	}
	return broker, os.Getenv("MQTT_TLS_CA_FILE"), os.Getenv("MQTT_TLS_CERT_FILE"), os.Getenv("MQTT_TLS_KEY_FILE")
}

// TestMQTTTLS_PublishAndReceive verifies that records are published over ssl:// with
// server verification and a client certificate, and that the subscriber receives them.
func TestMQTTTLS_PublishAndReceive(t *testing.T) {
	broker, caFile, certFile, keyFile := mqttTLSEnv(t)
	topic := uniqueTopic("mqtts")

	cfg := config.DefaultConfig()
	cfg.Backend = config.BackendMQTT
	cfg.MQTT.Broker = broker
	cfg.MQTT.Topic = topic
	cfg.MQTT.ClientID = fmt.Sprintf("it-mqtts-%d", time.Now().UnixNano())
	cfg.MQTT.TLSCACert = caFile
	cfg.MQTT.TLSClientCert = certFile
	cfg.MQTT.TLSClientKey = keyFile

	subTLS := tlsClientConfig(t, caFile, certFile, keyFile)
	received := mqttSubscribeTLS(t, broker, topic, subTLS)

	pub, err := mqttpub.NewPublisher(cfg)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	t.Cleanup(func() { _ = pub.Close() }) //nolint:errcheck
	if err := pub.PublishRecord(time.Now(), map[string]interface{}{"message": "mqtts-marker"}, ""); err != nil {
		t.Fatalf("PublishRecord over MQTTS: %v", err)
	}
	if body := string(mqttWaitFor(t, received)); !strings.Contains(body, "mqtts-marker") {
		t.Errorf("unexpected payload: %s", body)
	}
}

// TestMQTTTLS_RequiresClientCert verifies that the MQTTS broker rejects a publisher
// that presents no client certificate.
func TestMQTTTLS_RequiresClientCert(t *testing.T) {
	broker, caFile, _, _ := mqttTLSEnv(t)

	cfg := config.DefaultConfig()
	cfg.Backend = config.BackendMQTT
	cfg.MQTT.Broker = broker
	cfg.MQTT.Topic = uniqueTopic("mqtts-reject")
	cfg.MQTT.ClientID = fmt.Sprintf("it-mqtts-reject-%d", time.Now().UnixNano())
	cfg.MQTT.TLSCACert = caFile

	pub, err := mqttpub.NewPublisher(cfg)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	t.Cleanup(func() { _ = pub.Close() }) //nolint:errcheck
	if err := pub.PublishRecord(time.Now(), map[string]interface{}{"message": "rejected"}, ""); err == nil {
		t.Fatal("expected publish without a client certificate to be rejected")
	}
}

// mqttSubscribeTLS subscribes to topic over ssl:// using the given TLS configuration.
func mqttSubscribeTLS(t *testing.T, broker, topic string, tlsConfig *tls.Config) <-chan []byte {
	t.Helper()
	received := make(chan []byte, 16)
	opts := paho.NewClientOptions().
		AddBroker(broker).
		SetClientID(fmt.Sprintf("it-sub-tls-%d", time.Now().UnixNano())).
		SetConnectTimeout(10 * time.Second).
		SetTLSConfig(tlsConfig)
	client := paho.NewClient(opts)
	if token := client.Connect(); !token.WaitTimeout(10*time.Second) || token.Error() != nil {
		t.Fatalf("TLS subscriber connect: %v", token.Error())
	}
	t.Cleanup(func() { client.Disconnect(250) })

	token := client.Subscribe(topic, 1, func(_ paho.Client, msg paho.Message) {
		received <- msg.Payload()
	})
	if !token.WaitTimeout(10*time.Second) || token.Error() != nil {
		t.Fatalf("TLS subscribe %s: %v", topic, token.Error())
	}
	return received
}

// TestMQTTQoSLevels verifies that messages are delivered at each QoS level.
func TestMQTTQoSLevels(t *testing.T) {
	broker := mqttBroker(t)
	for qos := 0; qos <= 2; qos++ {
		t.Run(fmt.Sprintf("qos%d", qos), func(t *testing.T) {
			topic := uniqueTopic(fmt.Sprintf("qos%d", qos))
			received := mqttSubscribe(t, broker, topic, byte(qos))

			pub := mqttPublisher(t, broker, topic, qos)
			if err := pub.PublishRecord(time.Now(), map[string]interface{}{"qos": qos}, ""); err != nil {
				t.Fatalf("PublishRecord at qos %d: %v", qos, err)
			}
			if body := string(mqttWaitFor(t, received)); !strings.Contains(body, fmt.Sprintf(`"qos":%d`, qos)) {
				t.Errorf("unexpected payload at qos %d: %s", qos, body)
			}
		})
	}
}
