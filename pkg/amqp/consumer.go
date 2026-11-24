package amqp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/rossigee/fluent-bit-amqp-plugin/pkg/config"
	"go.uber.org/zap"
)

const (
	initialReconnectBackoff = time.Second
	maxReconnectBackoff     = 30 * time.Second
)

// Consumer reads messages from an AMQP queue on a background goroutine and hands
// them out in batches. Messages stay unacknowledged until AckAll is called, so a
// broker connection lost before acknowledgement causes redelivery.
type Consumer struct {
	config *config.AMQPConfig
	buffer chan amqp.Delivery
	logger *zap.SugaredLogger
}

// NewConsumer creates a consumer for the queue described by cfg. It does not connect;
// call Start to begin consuming.
func NewConsumer(cfg *config.AMQPConfig) (*Consumer, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}
	if cfg.Queue == "" {
		return nil, errors.New("queue is required for the AMQP input")
	}
	return &Consumer{
		config: cfg,
		buffer: make(chan amqp.Delivery, cfg.PrefetchCount),
	}, nil
}

// SetLogger sets the logger for the consumer
func (c *Consumer) SetLogger(logger *zap.SugaredLogger) {
	c.logger = logger
}

// Start consumes in the background until ctx is cancelled, reconnecting with
// exponential backoff whenever the connection or channel is lost.
func (c *Consumer) Start(ctx context.Context) {
	go c.run(ctx)
}

func (c *Consumer) run(ctx context.Context) {
	backoff := initialReconnectBackoff
	for {
		connected, err := c.consumeOnce(ctx)
		if ctx.Err() != nil {
			return
		}
		if connected {
			backoff = initialReconnectBackoff
		}
		if c.logger != nil {
			c.logger.Errorw("AMQP consumer stopped; reconnecting", "error", err, "backoff", backoff.String())
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, maxReconnectBackoff)
	}
}

// consumeOnce connects, declares the topology and consumes until the channel closes
// or ctx is cancelled. It reports whether a consumer was established.
func (c *Consumer) consumeOnce(ctx context.Context) (bool, error) {
	conn, err := Dial(c.config)
	if err != nil {
		return false, err
	}
	// Close errors are expected when the broker has already dropped the connection,
	// so they are logged at debug level.
	defer func() {
		if closeErr := conn.Close(); closeErr != nil && c.logger != nil {
			c.logger.Debugw("AMQP connection already closed", "error", closeErr)
		}
	}()

	ch, err := conn.Channel()
	if err != nil {
		return false, fmt.Errorf("failed to open AMQP channel: %w", err)
	}
	defer func() {
		if closeErr := ch.Close(); closeErr != nil && c.logger != nil {
			c.logger.Debugw("AMQP channel already closed", "error", closeErr)
		}
	}()

	if err := DeclareQueue(ch, c.config); err != nil {
		return false, err
	}
	if c.config.Exchange != "" {
		if err := ch.QueueBind(c.config.Queue, c.config.RoutingKey, c.config.Exchange, false, nil); err != nil {
			return false, fmt.Errorf("failed to bind queue: %w", err)
		}
	}
	if err := ch.Qos(c.config.PrefetchCount, 0, false); err != nil {
		return false, fmt.Errorf("failed to set prefetch: %w", err)
	}

	deliveries, err := ch.Consume(c.config.Queue, "", false, false, false, false, nil)
	if err != nil {
		return false, fmt.Errorf("failed to start consumer: %w", err)
	}
	if c.logger != nil {
		c.logger.Infow("AMQP consumer started", "queue", c.config.Queue, "prefetch_count", c.config.PrefetchCount)
	}

	for {
		select {
		case <-ctx.Done():
			return true, nil
		case d, ok := <-deliveries:
			if !ok {
				return true, errors.New("delivery channel closed")
			}
			select {
			case c.buffer <- d:
			case <-ctx.Done():
				return true, nil
			}
		}
	}
}

// Next returns up to max buffered deliveries without blocking. The caller must
// acknowledge them with AckAll once they have been handed on.
func (c *Consumer) Next(limit int) []amqp.Delivery {
	var batch []amqp.Delivery
	for len(batch) < limit {
		select {
		case d := <-c.buffer:
			batch = append(batch, d)
		default:
			return batch
		}
	}
	return batch
}

// AckAll acknowledges every delivery in batch.
func AckAll(batch []amqp.Delivery) error {
	var errs []error
	for _, d := range batch {
		if err := d.Ack(false); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// RecordFromBody converts a message body into a record for Fluent Bit. A JSON object
// is used as-is. When decodeCloudEvents is set and the body is a CloudEvent, the
// event's data is used. Any other body is wrapped as {"message": <body>}.
func RecordFromBody(body []byte, decodeCloudEvents bool) map[string]interface{} {
	var v interface{}
	if err := json.Unmarshal(body, &v); err == nil {
		if obj, ok := v.(map[string]interface{}); ok {
			if decodeCloudEvents {
				if _, isEvent := obj["specversion"]; isEvent {
					return cloudEventRecord(obj["data"])
				}
			}
			return obj
		}
	}
	return map[string]interface{}{"message": string(body)}
}

// cloudEventRecord turns a CloudEvent's data into a record.
func cloudEventRecord(data interface{}) map[string]interface{} {
	if obj, ok := data.(map[string]interface{}); ok {
		return obj
	}
	return map[string]interface{}{"data": data}
}
