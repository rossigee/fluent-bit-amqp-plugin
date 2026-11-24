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
	"github.com/rossigee/fluent-bit-amqp-plugin/pkg/payload"
	"go.uber.org/zap"
)

// Publisher handles AMQP message publishing
type Publisher struct {
	config     *config.AMQPConfig
	connection *amqp.Connection
	channel    *amqp.Channel
	returns    chan amqp.Return
	logger     *zap.SugaredLogger
}

// NewPublisher creates a new AMQP publisher with the given configuration
func NewPublisher(cfg *config.AMQPConfig) (*Publisher, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}
	p := &Publisher{config: cfg}
	return p, nil
}

// queueArguments returns the queue declaration arguments for the configured queue type and mode.
// Classic queues in default mode send no arguments, so existing declarations are unchanged.
func queueArguments(cfg *config.AMQPConfig) amqp.Table {
	args := amqp.Table{}
	if !cfg.IsClassicQueue() {
		args[amqp.QueueTypeArg] = cfg.QueueType
	}
	if cfg.QueueMode == config.QueueModeLazy {
		args["x-queue-mode"] = cfg.QueueMode
	}
	if len(args) == 0 {
		return nil
	}
	return args
}

// messageProperties returns the publishing properties shared by every message.
func messageProperties(cfg *config.AMQPConfig) amqp.Publishing {
	deliveryMode := amqp.Transient
	if cfg.Persistent {
		deliveryMode = amqp.Persistent
	}
	props := amqp.Publishing{
		ContentType:  cfg.ContentType,
		DeliveryMode: deliveryMode,
		Type:         cfg.MessageType,
		AppId:        cfg.AppID,
		Priority:     uint8(cfg.Priority), // #nosec G115 - range validated by config.Validate
	}
	if cfg.Expiration > 0 {
		props.Expiration = strconv.Itoa(cfg.Expiration)
	}
	return props
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
	conn, err := Dial(p.config)
	if err != nil {
		return err
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
		if err := DeclareQueue(ch, p.config); err != nil {
			if closeErr := ch.Close(); closeErr != nil {
				p.logError("Failed to close channel during cleanup", closeErr)
			}
			if closeErr := conn.Close(); closeErr != nil {
				p.logError("Failed to close connection during cleanup", closeErr)
			}
			return err
		}
	}

	// Publisher confirms let the broker report whether it accepted each message.
	if err := ch.Confirm(false); err != nil {
		if closeErr := ch.Close(); closeErr != nil {
			p.logError("Failed to close channel during cleanup", closeErr)
		}
		if closeErr := conn.Close(); closeErr != nil {
			p.logError("Failed to close connection during cleanup", closeErr)
		}
		return fmt.Errorf("failed to enable publisher confirms: %w", err)
	}

	p.connection = conn
	p.channel = ch
	// Buffered so returned messages never block the client while a publish waits.
	p.returns = ch.NotifyReturn(make(chan amqp.Return, 16))

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

// Dial opens a connection to the broker described by cfg, applying password
// encoding and, when enabled, the TLS settings.
func Dial(cfg *config.AMQPConfig) (*amqp.Connection, error) {
	urlStr, err := encodeURLPassword(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("failed to encode URL: %w", err)
	}

	var conn *amqp.Connection
	if cfg.TLS {
		tlsConfig, tlsErr := buildTLSConfig(cfg)
		if tlsErr != nil {
			return nil, fmt.Errorf("failed to build TLS config: %w", tlsErr)
		}
		conn, err = amqp.DialConfig(urlStr, amqp.Config{
			TLSClientConfig: tlsConfig,
		})
	} else {
		conn, err = amqp.Dial(urlStr)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to connect to AMQP server: %w", err)
	}
	return conn, nil
}

// DeclareQueue declares the configured queue on ch using the queue settings in cfg.
func DeclareQueue(ch *amqp.Channel, cfg *config.AMQPConfig) error {
	if _, err := ch.QueueDeclare(
		cfg.Queue,
		cfg.Durable,
		cfg.AutoDelete,
		cfg.Exclusive,
		cfg.NoWait,
		queueArguments(cfg),
	); err != nil {
		return fmt.Errorf("failed to declare queue: %w", err)
	}
	return nil
}

// buildTLSConfig builds a TLS configuration from the AMQP config
func buildTLSConfig(cfg *config.AMQPConfig) (*tls.Config, error) {
	// #nosec G402 - TLSInsecureSkipVerify is intentionally configurable for development/testing
	tlsConfig := &tls.Config{
		InsecureSkipVerify: cfg.TLSInsecureSkipVerify,
	}

	if cfg.TLSCertFile != "" && cfg.TLSKeyFile != "" {
		cert, err := tls.LoadX509KeyPair(cfg.TLSCertFile, cfg.TLSKeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load client certificate: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	if cfg.TLSCAFile != "" {
		caCert, err := os.ReadFile(cfg.TLSCAFile)
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

// buildTLSConfig builds a TLS configuration from the publisher's AMQP config
func (p *Publisher) buildTLSConfig() (*tls.Config, error) {
	return buildTLSConfig(p.config)
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

// EnsureConnection is the public wrapper for ensureConnection
func (p *Publisher) EnsureConnection() error {
	return p.ensureConnection()
}

// publishOnce attempts a single publish of a CloudEvent without any retry logic.
func (p *Publisher) publishOnce(event *cloudevents.Event) error {
	eventBytes, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal CloudEvent: %w", err)
	}

	props := messageProperties(p.config)
	props.Body = eventBytes
	props.Headers = amqp.Table{
		"ce-specversion": event.SpecVersion(),
		"ce-type":        event.Type(),
		"ce-source":      event.Source(),
		"ce-id":          event.ID(),
	}
	return p.publishConfirmed(props)
}

// publishConfirmed publishes with the mandatory flag and waits for the broker's
// confirm. It returns an error if the broker nacks the message or cannot route
// it to any queue, so a dropped message is never reported as published.
func (p *Publisher) publishConfirmed(props amqp.Publishing) error {
	ctx := context.Background()
	confirmation, err := p.channel.PublishWithDeferredConfirmWithContext(
		ctx,
		p.config.Exchange,
		p.config.RoutingKey,
		true,
		false,
		props,
	)
	if err != nil {
		return fmt.Errorf("failed to publish to AMQP: %w", err)
	}
	acked, err := confirmation.WaitContext(ctx)
	if err != nil {
		return fmt.Errorf("failed waiting for AMQP publisher confirm: %w", err)
	}
	if !acked {
		return fmt.Errorf("broker nacked message exchange=%q routing_key=%q", p.config.Exchange, p.config.RoutingKey)
	}
	// The broker sends basic.return before basic.ack for an unroutable message,
	// so any return for this publish is already queued once the ack arrives.
	select {
	case r := <-p.returns:
		return fmt.Errorf("message unroutable exchange=%q routing_key=%q reply_code=%d reply_text=%q",
			r.Exchange, r.RoutingKey, r.ReplyCode, r.ReplyText)
	default:
		return nil
	}
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

// normalizeRecord is payload.Normalize, kept under its package-local name for the tests.
func normalizeRecord(v interface{}) interface{} {
	return payload.Normalize(v)
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
	body, err := payload.Record(timestamp, record, tag)
	if err != nil {
		return err
	}

	props := messageProperties(p.config)
	props.Body = body
	return p.publishConfirmed(props)
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
