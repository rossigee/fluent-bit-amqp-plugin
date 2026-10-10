//go:build e2e

package e2e

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	amqplib "github.com/rabbitmq/amqp091-go"
)

const (
	e2eQueue  = "e2e-events"
	e2eMarker = "e2e-marker-record"
	e2eWait   = 60 * time.Second
)

// getOne fetches one message from the queue using a fresh connection. A queue that
// does not exist yet closes the channel, so each attempt needs its own connection.
func getOne(url string) (*amqplib.Delivery, error) {
	conn, err := amqplib.Dial(url)
	if err != nil {
		return nil, err
	}
	defer conn.Close() //nolint:errcheck

	ch, err := conn.Channel()
	if err != nil {
		return nil, err
	}
	defer ch.Close() //nolint:errcheck

	msg, ok, err := ch.Get(e2eQueue, true)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}
	return &msg, nil
}

// TestFluentBitPublishesToRabbitMQ checks that Fluent Bit, running the repo's image with the
// AMQP plugin loaded, publishes records to RabbitMQ with the plugin's default properties.
func TestFluentBitPublishesToRabbitMQ(t *testing.T) {
	url := os.Getenv("E2E_RABBITMQ_URL")
	if url == "" {
		url = "amqp://e2e:e2e-password@localhost:5672/"
	}

	deadline := time.Now().Add(e2eWait)
	for {
		msg, err := getOne(url)
		if err != nil {
			t.Fatalf("reading queue %s: %v", e2eQueue, err)
		}
		if msg != nil {
			checkMessage(t, msg)
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("no message on queue %s within %s", e2eQueue, e2eWait)
		}
		time.Sleep(time.Second)
	}
}

func checkMessage(t *testing.T, msg *amqplib.Delivery) {
	t.Helper()

	if !strings.Contains(string(msg.Body), e2eMarker) {
		t.Fatalf("message does not contain dummy record marker: %s", msg.Body)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(msg.Body, &payload); err != nil {
		t.Fatalf("body is not JSON: %v\nbody: %s", err, msg.Body)
	}
	if _, ok := payload["@timestamp"]; !ok {
		t.Errorf("payload missing @timestamp: %s", msg.Body)
	}
	if payload["tag"] != "e2e.dummy" {
		t.Errorf("tag: got %v, want e2e.dummy", payload["tag"])
	}

	// Plugin defaults: persistent delivery, app_id and JSON content type
	if msg.DeliveryMode != amqplib.Persistent {
		t.Errorf("DeliveryMode: got %d, want %d (persistent)", msg.DeliveryMode, amqplib.Persistent)
	}
	if msg.AppId != "fluent-bit" {
		t.Errorf("AppId: got %q, want fluent-bit", msg.AppId)
	}
	if msg.ContentType != "application/json" {
		t.Errorf("ContentType: got %q, want application/json", msg.ContentType)
	}
}
