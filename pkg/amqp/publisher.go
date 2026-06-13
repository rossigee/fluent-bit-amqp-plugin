package amqp

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/rossigee/fluent-bit-amqp-plugin/pkg/config"
	"go.uber.org/zap"
)

// Publisher handles AMQP message publishing
type Publisher struct {
	config     *config.AMQPConfig
	connection *amqp.Connection
	channel    *amqp.Channel
	logger     *zap.SugaredLogger
}

// NewPublisher creates a new AMQP publisher with the given configuration
func NewPublisher(cfg *config.AMQPConfig) (*Publisher, error) {
	p := &Publisher{config: cfg}
	return p, nil
}

// SetLogger sets the logger for the publisher
func (p *Publisher) SetLogger(logger *zap.SugaredLogger) {
	p.logger = logger
}

func (p *Publisher) logError(msg string, err error) {
	if p.logger != nil {
		p.logger.Errorw(msg, "error", err)
	}
}

func (p *Publisher) logInfo(msg string, fields ...interface{}) {
	if p.logger != nil {
		p.logger.Infow(msg, fields...)
	}
}

// connect establishes connection and channel to AMQP broker
func (p *Publisher) connect() error {
	var conn *amqp.Connection
	var err error

	urlStr, err := encodeURLPassword(p.config.URL)
	if err != nil {
		return fmt.Errorf("failed to encode URL: %w", err)
	}

	if p.config.TLS {
		tlsConfig, tlsErr := p.buildTLSConfig()
		if tlsErr != nil {
			return fmt.Errorf("failed to build TLS config: %w", tlsErr)
		}
		conn, err = amqp.DialConfig(urlStr, amqp.Config{
			TLSClientConfig: tlsConfig,
		})
	} else {
		conn, err = amqp.Dial(urlStr)
	}

	if err != nil {
		return fmt.Errorf("failed to connect to AMQP server: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		if closeErr := conn.Close(); closeErr != nil {
			p.logError("Failed to close connection during cleanup", closeErr)
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
				p.logError("Failed to close channel during cleanup", closeErr)
			}
			if closeErr := conn.Close(); closeErr != nil {
				p.logError("Failed to close connection during cleanup", closeErr)
			}
			return fmt.Errorf("failed to declare queue: %w", err)
		}
	}

	p.connection = conn
	p.channel = ch

	if p.logger != nil {
		p.logger.Infow("AMQP connection established", "queue", p.config.Queue)
	}
	return nil
}

// encodeURLPassword URL-encodes the password in an AMQP URL to handle special characters.
// This is necessary because some credential rotation systems generate passwords with
// characters like +, =, / that need encoding in URLs.
func encodeURLPassword(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse URL: %w", err)
	}

	user := u.User
	if user == nil {
		return rawURL, nil
	}

	password, hasPassword := user.Password()
	if !hasPassword || password == "" {
		return rawURL, nil
	}

	encodedPassword := encodeUserInfoPassword(password)

	return strings.Replace(rawURL, password, encodedPassword, 1), nil
}

// encodeUserInfoPassword encodes special characters in a password for use in URL userinfo.
// Characters +, =, /, % have special meaning in URLs and must be percent-encoded.
// If the password is already URL-encoded (contains % but not as part of valid encoding),
// it is returned as-is to avoid double-encoding.
func encodeUserInfoPassword(password string) string {
	if strings.Contains(password, "%") && !isValidEncodedPassword(password) {
		return password
	}
	var sb strings.Builder
	for _, c := range password {
		switch c {
		case '+':
			sb.WriteString("%2B")
		case '=':
			sb.WriteString("%3D")
		case '/':
			sb.WriteString("%2F")
		case '%':
			sb.WriteString("%25")
		default:
			sb.WriteRune(c)
		}
	}
	return sb.String()
}

func isValidEncodedPassword(password string) bool {
	if len(password) < 3 {
		return false
	}
	for i := 0; i < len(password)-2; i++ {
		if password[i] == '%' {
			hex := password[i+1 : i+3]
			if _, err := strconv.ParseInt(hex, 16, 16); err != nil {
				return false
			}
		}
	}
	return true
}

// buildTLSConfig builds a TLS configuration from the AMQP config
func (p *Publisher) buildTLSConfig() (*tls.Config, error) {
	// #nosec G402 - TLSInsecureSkipVerify is intentionally configurable for development/testing
	tlsConfig := &tls.Config{
		InsecureSkipVerify: p.config.TLSInsecureSkipVerify,
	}

	if p.config.TLSCertFile != "" && p.config.TLSKeyFile != "" {
		cert, err := tls.LoadX509KeyPair(p.config.TLSCertFile, p.config.TLSKeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load client certificate: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	if p.config.TLSCAFile != "" {
		caCert, err := os.ReadFile(p.config.TLSCAFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA certificate: %w", err)
		}
		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to parse CA certificate")
		}
		tlsConfig.RootCAs = caCertPool
	}

	return tlsConfig, nil
}

// PublishCloudEvent publishes a CloudEvent to the AMQP broker, reconnecting once on a closed connection.
func (p *Publisher) PublishCloudEvent(event *cloudevents.Event) error {
	if err := p.ensureConnection(); err != nil {
		return err
	}

	err := p.publishOnce(event)
	if err == nil {
		return nil
	}
	if !errors.Is(err, amqp.ErrClosed) {
		return err
	}
	if reconnectErr := p.reconnect(); reconnectErr != nil {
		return fmt.Errorf("failed to reconnect: %w", reconnectErr)
	}
	return p.publishOnce(event)
}

// ensureConnection connects if not already connected
func (p *Publisher) ensureConnection() error {
	if p.connection != nil && !p.connection.IsClosed() {
		return nil
	}
	return p.connect()
}

// publishOnce attempts a single publish of a CloudEvent without any retry logic.
func (p *Publisher) publishOnce(event *cloudevents.Event) error {
	eventBytes, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal CloudEvent: %w", err)
	}

	err = p.channel.PublishWithContext(
		context.Background(),
		p.config.Exchange,
		p.config.RoutingKey,
		false,
		false,
		amqp.Publishing{
			ContentType: "application/json",
			Body:        eventBytes,
			Headers: amqp.Table{
				"ce-specversion": event.SpecVersion(),
				"ce-type":        event.Type(),
				"ce-source":      event.Source(),
				"ce-id":          event.ID(),
			},
		},
	)
	if err != nil {
		return fmt.Errorf("failed to publish to AMQP: %w", err)
	}
	return nil
}

// reconnect attempts to re-establish the AMQP connection
func (p *Publisher) reconnect() error {
	// Close existing connections
	if p.channel != nil {
		if closeErr := p.channel.Close(); closeErr != nil {
			p.logError("Failed to close channel during reconnect", closeErr)
		}
	}
	if p.connection != nil {
		if closeErr := p.connection.Close(); closeErr != nil {
			p.logError("Failed to close connection during reconnect", closeErr)
		}
	}

	// Reconnect
	if err := p.connect(); err != nil {
		return fmt.Errorf("failed to reconnect: %w", err)
	}

	p.logInfo("Successfully reconnected to AMQP")
	return nil
}

// normalizeRecord converts map[interface{}]interface{} to map[string]interface{}
// recursively, which is required for JSON marshaling support.
func normalizeRecord(v interface{}) interface{} {
	switch v := v.(type) {
	case map[interface{}]interface{}:
		result := make(map[string]interface{}, len(v))
		for key, value := range v {
			strKey := fmt.Sprintf("%v", key)
			result[strKey] = normalizeRecord(value)
		}
		return result
	case []interface{}:
		result := make([]interface{}, len(v))
		for i, item := range v {
			result[i] = normalizeRecord(item)
		}
		return result
	default:
		return v
	}
}

// PublishRecord publishes a raw Fluent Bit record to the AMQP broker as plain
// JSON, reconnecting once on a closed connection. The message body is a JSON
// object containing the record fields plus "@timestamp" and "tag" keys.
func (p *Publisher) PublishRecord(timestamp time.Time, record interface{}, tag string) error {
	if err := p.ensureConnection(); err != nil {
		return err
	}

	err := p.publishRecordOnce(timestamp, record, tag)
	if err == nil {
		return nil
	}
	if !errors.Is(err, amqp.ErrClosed) {
		return err
	}
	if reconnectErr := p.reconnect(); reconnectErr != nil {
		return fmt.Errorf("failed to reconnect: %w", reconnectErr)
	}
	return p.publishRecordOnce(timestamp, record, tag)
}

// publishRecordOnce marshals and publishes a single record without retry logic.
func (p *Publisher) publishRecordOnce(timestamp time.Time, record interface{}, tag string) error {
	normalized, ok := normalizeRecord(record).(map[string]interface{})
	if !ok {
		normalized = map[string]interface{}{"data": normalizeRecord(record)}
	}

	// Merge metadata into a shallow copy so we don't mutate the original.
	payload := make(map[string]interface{}, len(normalized)+2)
	for k, v := range normalized {
		payload[k] = v
	}
	payload["@timestamp"] = timestamp.UTC().Format(time.RFC3339)
	if tag != "" {
		payload["tag"] = tag
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal record: %w", err)
	}

	err = p.channel.PublishWithContext(
		context.Background(),
		p.config.Exchange,
		p.config.RoutingKey,
		false,
		false,
		amqp.Publishing{
			ContentType: "application/json",
			Body:        body,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to publish to AMQP: %w", err)
	}
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
	p.channel = nil
	p.connection = nil
	p.logInfo("AMQP connection closed")
	return nil
}
