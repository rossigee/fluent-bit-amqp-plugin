package config

import (
	"fmt"
	"strconv"
	"unsafe"

	"github.com/fluent/fluent-bit-go/output"
)

// Queue types accepted by queue_type
const (
	QueueTypeClassic = "classic"
	QueueTypeQuorum  = "quorum"
	QueueTypeStream  = "stream"
)

// Queue modes accepted by queue_mode
const (
	QueueModeDefault = "default"
	QueueModeLazy    = "lazy"
)

// AMQPConfig holds the configuration for the AMQP output plugin
type AMQPConfig struct {
	URL                   string
	Exchange              string
	RoutingKey            string
	Queue                 string
	CloudEvents           bool
	EventSource           string
	EventType             string
	Durable               bool
	AutoDelete            bool
	Exclusive             bool
	NoWait                bool
	TLS                   bool
	TLSInsecureSkipVerify bool
	TLSCAFile             string
	TLSCertFile           string
	TLSKeyFile            string
	ConnectOnStartup      bool
	QueueType             string
	QueueMode             string
	Persistent            bool
	Expiration            int
	ContentType           string
	MessageType           string
	Priority              int
	AppID                 string
}

// DefaultConfig returns a new AMQPConfig with default values
func DefaultConfig() *AMQPConfig {
	return &AMQPConfig{
		URL:                   "amqp://localhost:5672/",
		Exchange:              "",
		RoutingKey:            "fluent-bit-events",
		Queue:                 "fluent-bit-events",
		CloudEvents:           true,
		EventSource:           "fluent-bit",
		EventType:             "fluent-bit.log",
		Durable:               true,
		AutoDelete:            false,
		Exclusive:             false,
		NoWait:                false,
		TLS:                   false,
		TLSInsecureSkipVerify: false,
		TLSCAFile:             "",
		TLSCertFile:           "",
		TLSKeyFile:            "",
		ConnectOnStartup:      true,
		QueueType:             QueueTypeClassic,
		QueueMode:             QueueModeDefault,
		Persistent:            true,
		Expiration:            0,
		ContentType:           "application/json",
		MessageType:           "",
		Priority:              0,
		AppID:                 "fluent-bit",
	}
}

// Validate checks that the configuration is one RabbitMQ will accept
func (c *AMQPConfig) Validate() error {
	switch c.QueueType {
	case "", QueueTypeClassic, QueueTypeQuorum, QueueTypeStream:
	default:
		return fmt.Errorf("invalid queue_type %q: must be %s, %s or %s", c.QueueType, QueueTypeClassic, QueueTypeQuorum, QueueTypeStream)
	}

	switch c.QueueMode {
	case "", QueueModeDefault, QueueModeLazy:
	default:
		return fmt.Errorf("invalid queue_mode %q: must be %s or %s", c.QueueMode, QueueModeDefault, QueueModeLazy)
	}

	if c.IsClassicQueue() {
		return c.validateExpirationAndPriority()
	}

	if c.QueueMode == QueueModeLazy {
		return fmt.Errorf("queue_mode %s only applies to classic queues, not %s", QueueModeLazy, c.QueueType)
	}
	if !c.Durable {
		return fmt.Errorf("queue_type %s requires durable=true", c.QueueType)
	}
	if c.Exclusive {
		return fmt.Errorf("queue_type %s does not support exclusive=true", c.QueueType)
	}
	if c.AutoDelete {
		return fmt.Errorf("queue_type %s does not support auto_delete=true", c.QueueType)
	}

	return c.validateExpirationAndPriority()
}

// IsClassicQueue reports whether the configured queue type is classic (the broker default)
func (c *AMQPConfig) IsClassicQueue() bool {
	return c.QueueType == "" || c.QueueType == QueueTypeClassic
}

func (c *AMQPConfig) validateExpirationAndPriority() error {
	if c.Expiration < 0 {
		return fmt.Errorf("invalid expiration %d: must be 0 (disabled) or a positive number of milliseconds", c.Expiration)
	}
	if c.Priority < 0 || c.Priority > 255 {
		return fmt.Errorf("invalid priority %d: must be between 0 and 255", c.Priority)
	}
	return nil
}

// LoadFromFluentBit loads configuration from Fluent Bit plugin context
func (c *AMQPConfig) LoadFromFluentBit(plugin unsafe.Pointer) error {
	if url := output.FLBPluginConfigKey(plugin, "url"); url != "" {
		c.URL = url
	}
	if exchange := output.FLBPluginConfigKey(plugin, "exchange"); exchange != "" {
		c.Exchange = exchange
	}
	if routingKey := output.FLBPluginConfigKey(plugin, "routing_key"); routingKey != "" {
		c.RoutingKey = routingKey
	}
	if queue := output.FLBPluginConfigKey(plugin, "queue"); queue != "" {
		c.Queue = queue
	}
	if cloudEvents := output.FLBPluginConfigKey(plugin, "cloudevents"); cloudEvents != "" {
		c.CloudEvents, _ = strconv.ParseBool(cloudEvents)
	}
	if eventSource := output.FLBPluginConfigKey(plugin, "event_source"); eventSource != "" {
		c.EventSource = eventSource
	}
	if eventType := output.FLBPluginConfigKey(plugin, "event_type"); eventType != "" {
		c.EventType = eventType
	}
	if durable := output.FLBPluginConfigKey(plugin, "durable"); durable != "" {
		c.Durable, _ = strconv.ParseBool(durable)
	}
	if autoDelete := output.FLBPluginConfigKey(plugin, "auto_delete"); autoDelete != "" {
		c.AutoDelete, _ = strconv.ParseBool(autoDelete)
	}
	if exclusive := output.FLBPluginConfigKey(plugin, "exclusive"); exclusive != "" {
		c.Exclusive, _ = strconv.ParseBool(exclusive)
	}
	if noWait := output.FLBPluginConfigKey(plugin, "no_wait"); noWait != "" {
		c.NoWait, _ = strconv.ParseBool(noWait)
	}
	if tls := output.FLBPluginConfigKey(plugin, "tls"); tls != "" {
		c.TLS, _ = strconv.ParseBool(tls)
	}
	if tlsInsecureSkipVerify := output.FLBPluginConfigKey(plugin, "tls_insecure_skip_verify"); tlsInsecureSkipVerify != "" {
		c.TLSInsecureSkipVerify, _ = strconv.ParseBool(tlsInsecureSkipVerify)
	}
	if tlsCAFile := output.FLBPluginConfigKey(plugin, "tls_ca_file"); tlsCAFile != "" {
		c.TLSCAFile = tlsCAFile
	}
	if tlsCertFile := output.FLBPluginConfigKey(plugin, "tls_cert_file"); tlsCertFile != "" {
		c.TLSCertFile = tlsCertFile
	}
	if tlsKeyFile := output.FLBPluginConfigKey(plugin, "tls_key_file"); tlsKeyFile != "" {
		c.TLSKeyFile = tlsKeyFile
	}
	if connectOnStartup := output.FLBPluginConfigKey(plugin, "connect_on_startup"); connectOnStartup != "" {
		c.ConnectOnStartup, _ = strconv.ParseBool(connectOnStartup)
	}
	if queueType := output.FLBPluginConfigKey(plugin, "queue_type"); queueType != "" {
		c.QueueType = queueType
	}
	if queueMode := output.FLBPluginConfigKey(plugin, "queue_mode"); queueMode != "" {
		c.QueueMode = queueMode
	}
	if persistent := output.FLBPluginConfigKey(plugin, "persistent"); persistent != "" {
		v, err := strconv.ParseBool(persistent)
		if err != nil {
			return fmt.Errorf("invalid persistent %q: %w", persistent, err)
		}
		c.Persistent = v
	}
	if expiration := output.FLBPluginConfigKey(plugin, "expiration"); expiration != "" {
		v, err := strconv.Atoi(expiration)
		if err != nil {
			return fmt.Errorf("invalid expiration %q: %w", expiration, err)
		}
		c.Expiration = v
	}
	if contentType := output.FLBPluginConfigKey(plugin, "content_type"); contentType != "" {
		c.ContentType = contentType
	}
	if messageType := output.FLBPluginConfigKey(plugin, "message_type"); messageType != "" {
		c.MessageType = messageType
	}
	if priority := output.FLBPluginConfigKey(plugin, "priority"); priority != "" {
		v, err := strconv.Atoi(priority)
		if err != nil {
			return fmt.Errorf("invalid priority %q: %w", priority, err)
		}
		c.Priority = v
	}
	if appID := output.FLBPluginConfigKey(plugin, "app_id"); appID != "" {
		c.AppID = appID
	}
	return nil
}
