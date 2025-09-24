package amqp

import (
	"encoding/json"
	"fmt"
	"log"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/rossigee/fluent-bit-amqp-plugin/pkg/config"
	"github.com/streadway/amqp"
)

// Publisher handles AMQP message publishing
type Publisher struct {
	config     *config.AMQPConfig
	connection *amqp.Connection
	channel    *amqp.Channel
}

// NewPublisher creates a new AMQP publisher with the given configuration
func NewPublisher(cfg *config.AMQPConfig) (*Publisher, error) {
	p := &Publisher{config: cfg}

	if err := p.connect(); err != nil {
		return nil, fmt.Errorf("failed to initialize AMQP connection: %w", err)
	}

	return p, nil
}

// connect establishes connection and channel to AMQP broker
func (p *Publisher) connect() error {
	// Connect to AMQP server
	conn, err := amqp.Dial(p.config.URL)
	if err != nil {
		return fmt.Errorf("failed to connect to AMQP server: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		if closeErr := conn.Close(); closeErr != nil {
			log.Printf("Failed to close connection during cleanup: %v", closeErr)
		}
		return fmt.Errorf("failed to open AMQP channel: %w", err)
	}

	// Declare queue if specified
	if p.config.Queue != "" {
		_, err = ch.QueueDeclare(
			p.config.Queue,
			p.config.Durable,
			p.config.AutoDelete,
			p.config.Exclusive,
			p.config.NoWait,
			nil,
		)
		if err != nil {
			if closeErr := ch.Close(); closeErr != nil {
				log.Printf("Failed to close channel during cleanup: %v", closeErr)
			}
			if closeErr := conn.Close(); closeErr != nil {
				log.Printf("Failed to close connection during cleanup: %v", closeErr)
			}
			return fmt.Errorf("failed to declare queue: %w", err)
		}
	}

	p.connection = conn
	p.channel = ch

	log.Printf("AMQP connection established - URL: %s, Queue: %s", p.config.URL, p.config.Queue)
	return nil
}

// PublishCloudEvent publishes a CloudEvent to the AMQP broker
func (p *Publisher) PublishCloudEvent(event *cloudevents.Event) error {
	// Marshal CloudEvent to JSON
	eventBytes, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal CloudEvent: %w", err)
	}

	// Publish to AMQP
	err = p.channel.Publish(
		p.config.Exchange,   // exchange
		p.config.RoutingKey, // routing key
		false,               // mandatory
		false,               // immediate
		amqp.Publishing{
			ContentType: "application/json",
			Body:        eventBytes,
			Headers: amqp.Table{
				"ce-specversion": event.SpecVersion(),
				"ce-type":        event.Type(),
				"ce-source":      event.Source(),
				"ce-id":          event.ID(),
			},
		})

	if err != nil {
		// Try to reconnect if connection was lost
		if err == amqp.ErrClosed {
			if reconnectErr := p.reconnect(); reconnectErr != nil {
				return fmt.Errorf("failed to reconnect: %w", reconnectErr)
			}
			// Retry publishing
			return p.PublishCloudEvent(event)
		}
		return fmt.Errorf("failed to publish to AMQP: %w", err)
	}

	return nil
}

// reconnect attempts to re-establish the AMQP connection
func (p *Publisher) reconnect() error {
	// Close existing connections
	if p.channel != nil {
		if closeErr := p.channel.Close(); closeErr != nil {
			log.Printf("Failed to close channel during reconnect: %v", closeErr)
		}
	}
	if p.connection != nil {
		if closeErr := p.connection.Close(); closeErr != nil {
			log.Printf("Failed to close connection during reconnect: %v", closeErr)
		}
	}

	// Reconnect
	if err := p.connect(); err != nil {
		return fmt.Errorf("failed to reconnect: %w", err)
	}

	log.Printf("Successfully reconnected to AMQP")
	return nil
}

// Close closes the AMQP connection and channel
func (p *Publisher) Close() error {
	if p.channel != nil {
		if err := p.channel.Close(); err != nil {
			return fmt.Errorf("failed to close AMQP channel: %w", err)
		}
	}
	if p.connection != nil {
		if err := p.connection.Close(); err != nil {
			return fmt.Errorf("failed to close AMQP connection: %w", err)
		}
	}
	log.Printf("AMQP connection closed")
	return nil
}
