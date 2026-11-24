// Package mqtt publishes Fluent Bit records to an MQTT broker. It is the MQTT
// counterpart of the AMQP publisher and produces the same plain JSON and
// CloudEvents bodies.
package mqtt

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	paho "github.com/eclipse/paho.mqtt.golang"
	"github.com/rossigee/fluent-bit-amqp-plugin/pkg/config"
	"github.com/rossigee/fluent-bit-amqp-plugin/pkg/payload"
	"go.uber.org/zap"
)

const (
	connectTimeout = 10 * time.Second
	publishTimeout = 10 * time.Second
)

// Publisher publishes messages to an MQTT topic
type Publisher struct {
	config *config.AMQPConfig
	client paho.Client
	logger *zap.SugaredLogger
}

// NewPublisher creates an MQTT publisher for the configuration. It does not connect;
// the connection is opened on first use or by EnsureConnection.
func NewPublisher(cfg *config.AMQPConfig) (*Publisher, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}
	return &Publisher{config: cfg}, nil
}

// SetLogger sets the logger for the publisher
func (p *Publisher) SetLogger(logger *zap.SugaredLogger) {
	p.logger = logger
}

// EnsureConnection opens the connection to the broker if it is not already open.
func (p *Publisher) EnsureConnection() error {
	if p.client != nil {
		if p.client.IsConnectionOpen() {
			return nil
		}
		// The client was connected before and is reconnecting in the background. Creating
		// a second client with the same ID would make the broker drop one of them.
		return errors.New("MQTT connection is down; waiting for automatic reconnect")
	}
	return p.connect()
}

func (p *Publisher) connect() error {
	m := p.config.MQTT
	opts := paho.NewClientOptions().
		AddBroker(m.Broker).
		SetClientID(m.ClientID).
		SetUsername(m.Username).
		SetPassword(m.Password).
		SetKeepAlive(time.Duration(m.Keepalive) * time.Second).
		SetAutoReconnect(true).
		SetCleanSession(true)

	if strings.HasPrefix(m.Broker, "ssl://") || strings.HasPrefix(m.Broker, "wss://") {
		tlsConfig, err := buildTLSConfig(&m)
		if err != nil {
			return fmt.Errorf("failed to build TLS config: %w", err)
		}
		opts.SetTLSConfig(tlsConfig)
	}

	opts.SetOnConnectHandler(func(_ paho.Client) {
		if p.logger != nil {
			p.logger.Infow("MQTT connection established", "broker", m.Broker, "topic", m.Topic)
		}
	})
	opts.SetConnectionLostHandler(func(_ paho.Client, err error) {
		if p.logger != nil {
			p.logger.Errorw("MQTT connection lost; reconnecting automatically", "error", err)
		}
	})

	client := paho.NewClient(opts)
	token := client.Connect()
	if !token.WaitTimeout(connectTimeout) {
		return fmt.Errorf("timed out connecting to MQTT broker %s", m.Broker)
	}
	if err := token.Error(); err != nil {
		return fmt.Errorf("failed to connect to MQTT broker: %w", err)
	}
	p.client = client
	return nil
}

// PublishCloudEvent publishes a CloudEvent. The event's attributes are carried in the
// JSON body, as the CloudEvents JSON format specifies.
func (p *Publisher) PublishCloudEvent(event *cloudevents.Event) error {
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal CloudEvent: %w", err)
	}
	return p.publish(body)
}

// PublishRecord publishes a Fluent Bit record as plain JSON
func (p *Publisher) PublishRecord(timestamp time.Time, record interface{}, tag string) error {
	body, err := payload.Record(timestamp, record, tag)
	if err != nil {
		return err
	}
	return p.publish(body)
}

func (p *Publisher) publish(body []byte) error {
	if err := p.EnsureConnection(); err != nil {
		return err
	}
	// QoS is range-checked by config.Validate
	token := p.client.Publish(p.config.MQTT.Topic, byte(p.config.MQTT.QoS), p.config.MQTT.Retain, body) // #nosec G115
	if !token.WaitTimeout(publishTimeout) {
		return fmt.Errorf("timed out publishing to MQTT topic %s", p.config.MQTT.Topic)
	}
	if err := token.Error(); err != nil {
		return fmt.Errorf("failed to publish to MQTT: %w", err)
	}
	return nil
}

// Close disconnects from the broker
func (p *Publisher) Close() error {
	if p.client != nil {
		p.client.Disconnect(250)
		p.client = nil
	}
	if p.logger != nil {
		p.logger.Infow("MQTT connection closed")
	}
	return nil
}

// buildTLSConfig builds the TLS configuration for an ssl:// or wss:// broker. Server
// certificates are always verified. A CA file and an optional client certificate
// (mutual TLS) can be supplied.
func buildTLSConfig(m *config.MQTTConfig) (*tls.Config, error) {
	tlsConfig := &tls.Config{}

	if m.TLSCACert != "" {
		caPEM, err := os.ReadFile(m.TLSCACert)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA certificate: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caPEM) {
			return nil, errors.New("failed to parse CA certificate")
		}
		tlsConfig.RootCAs = pool
	}

	if m.TLSClientCert != "" && m.TLSClientKey != "" {
		cert, err := tls.LoadX509KeyPair(m.TLSClientCert, m.TLSClientKey)
		if err != nil {
			return nil, fmt.Errorf("failed to load client certificate: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	return tlsConfig, nil
}
