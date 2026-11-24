//go:build integration

package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	amqplib "github.com/rabbitmq/amqp091-go"
	"github.com/rossigee/fluent-bit-amqp-plugin/pkg/amqp"
	"github.com/rossigee/fluent-bit-amqp-plugin/pkg/config"
)

// TestConsumerReceivesAndAcks verifies that the input consumer delivers published
// messages in batches and that acknowledging them empties the queue.
func TestConsumerReceivesAndAcks(t *testing.T) {
	url := rabbitmqURL(t)
	queueName := fmt.Sprintf("integration-consume-%d", time.Now().UnixNano())

	// Publish the messages before the consumer starts, so they are waiting in the queue.
	setup, err := amqplib.Dial(url)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer setup.Close() //nolint:errcheck

	ch, err := setup.Channel()
	if err != nil {
		t.Fatalf("channel: %v", err)
	}
	defer ch.Close() //nolint:errcheck
	defer ch.QueueDelete(queueName, false, false, false) //nolint:errcheck

	if _, err := ch.QueueDeclare(queueName, true, false, false, false, nil); err != nil {
		t.Fatalf("QueueDeclare: %v", err)
	}
	bodies := []string{`{"message":"first"}`, `{"message":"second"}`, "third plain text"}
	for _, body := range bodies {
		if err := ch.PublishWithContext(context.Background(), "", queueName, false, false, amqplib.Publishing{
			ContentType: "application/json",
			Body:        []byte(body),
		}); err != nil {
			t.Fatalf("publish: %v", err)
		}
	}

	cfg := config.DefaultConfig()
	cfg.URL = url
	cfg.Queue = queueName
	cfg.RoutingKey = queueName
	cfg.Durable = true
	cfg.AutoDelete = false
	cfg.PrefetchCount = 10

	consumer, err := amqp.NewConsumer(cfg)
	if err != nil {
		t.Fatalf("NewConsumer: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	consumer.Start(ctx)

	// Poll the buffer until every message has arrived. The consumer is asynchronous,
	// so the deadline bounds the wait.
	var batch []amqplib.Delivery
	deadline := time.NewTimer(30 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for len(batch) < len(bodies) {
		select {
		case <-deadline.C:
			t.Fatalf("received %d of %d messages before timeout", len(batch), len(bodies))
		case <-ticker.C:
			batch = append(batch, consumer.Next(len(bodies)-len(batch))...)
		}
	}

	for i, d := range batch {
		if string(d.Body) != bodies[i] {
			t.Errorf("message %d body = %q, want %q", i, d.Body, bodies[i])
		}
	}
	if err := amqp.AckAll(batch); err != nil {
		t.Fatalf("AckAll: %v", err)
	}

	// Acknowledged messages leave the queue; poll until the broker has processed the acks.
	for {
		q, err := ch.QueueDeclarePassive(queueName, true, false, false, false, nil)
		if err != nil {
			t.Fatalf("QueueDeclarePassive: %v", err)
		}
		if q.Messages == 0 && q.Consumers == 1 {
			return
		}
		select {
		case <-deadline.C:
			t.Fatalf("queue still holds %d messages after ack", q.Messages)
		case <-ticker.C:
		}
	}
}
