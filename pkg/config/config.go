package config

import (
	"errors"
	"fmt"
	"net/url"
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
	// Input-only settings (ignored by the output plugin)
	PrefetchCount     int
	CloudEventsDecode bool
	// Backend selects the output transport: BackendAMQP (default) or BackendMQTT.
	// When BackendMQTT is set, the AMQP settings above are ignored and MQTT holds the settings.
	Backend string
	MQTT    MQTTConfig
}

// Transport backends accepted by backend
const (
	BackendAMQP = "amqp"
	BackendMQTT = "mqtt"
)

// MQTTConfig holds the settings for the MQTT output backend
type MQTTConfig struct {
	Broker        string
	Topic         string
	QoS           int
	Retain        bool
	ClientID      string
	Username      string
	Password      string
	Keepalive     int
	TLSCACert     string
	TLSClientCert string
	TLSClientKey  string
}

// Validate checks the MQTT settings
func (m *MQTTConfig) Validate() error {
	u, err := url.Parse(m.Broker)
	if err != nil {
		return fmt.Errorf("invalid broker %q: %w", m.Broker, err)
	}
	switch u.Scheme {
	case "tcp", "ssl", "ws", "wss":
	default:
		return fmt.Errorf("invalid broker %q: scheme must be tcp, ssl, ws or wss", m.Broker)
	}
	if u.Host == "" {
		return fmt.Errorf("invalid broker %q: missing host", m.Broker)
	}
	if m.Topic == "" {
		return errors.New("topic is required for the MQTT backend")
	}
	if m.QoS < 0 || m.QoS > 2 {
		return fmt.Errorf("invalid qos %d: must be 0, 1 or 2", m.QoS)
	}
	if m.Keepalive < 1 {
		return fmt.Errorf("invalid keepalive %d: must be at least 1 second", m.Keepalive)
	}
	if (m.TLSClientCert == "") != (m.TLSClientKey == "") {
		return errors.New("tls_client_cert and tls_client_key must be set together")
	}
	hasTLSFiles := m.TLSCACert != "" || m.TLSClientCert != ""
	if hasTLSFiles && u.Scheme != "ssl" && u.Scheme != "wss" {
		return fmt.Errorf("tls_ca_cert and tls_client_cert need an ssl:// or wss:// broker, not %s://", u.Scheme)
	}
	return nil
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
		PrefetchCount:         10,
		CloudEventsDecode:     false,
		Backend:               BackendAMQP,
		MQTT: MQTTConfig{
			Broker:    "tcp://localhost:1883",
			Topic:     "fluent-bit/logs",
			QoS:       1,
			Retain:    false,
			ClientID:  "fluent-bit",
			Keepalive: 30,
		},
	}
}

// Validate checks that the configuration is one the selected backend will accept
func (c *AMQPConfig) Validate() error {
	switch c.Backend {
	case "", BackendAMQP:
	case BackendMQTT:
		return c.MQTT.Validate()
	default:
		return fmt.Errorf("invalid backend %q: must be %s or %s", c.Backend, BackendAMQP, BackendMQTT)
	}

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
	if c.PrefetchCount < 1 {
		return fmt.Errorf("invalid prefetch_count %d: must be at least 1", c.PrefetchCount)
	}
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
	return c.loadFrom(func(key string) string {
		return output.FLBPluginConfigKey(plugin, key)
	})
}

// LoadFromLookup loads configuration, reading each key through lookup. An empty value means the key is unset.
func (c *AMQPConfig) LoadFromLookup(lookup func(key string) string) error {
	return c.loadFrom(lookup)
}

// loadFrom loads configuration, reading each key through lookup. An empty value means the key is unset.
func (c *AMQPConfig) loadFrom(lookup func(key string) string) error {
	if url := lookup("url"); url != "" {
		c.URL = url
	}
	if exchange := lookup("exchange"); exchange != "" {
		c.Exchange = exchange
	}
	if routingKey := lookup("routing_key"); routingKey != "" {
		c.RoutingKey = routingKey
	}
	if queue := lookup("queue"); queue != "" {
		c.Queue = queue
	}
	if cloudEvents := lookup("cloudevents"); cloudEvents != "" {
		c.CloudEvents, _ = strconv.ParseBool(cloudEvents)
	}
	if eventSource := lookup("event_source"); eventSource != "" {
		c.EventSource = eventSource
	}
	if eventType := lookup("event_type"); eventType != "" {
		c.EventType = eventType
	}
	if durable := lookup("durable"); durable != "" {
		c.Durable, _ = strconv.ParseBool(durable)
	}
	if autoDelete := lookup("auto_delete"); autoDelete != "" {
		c.AutoDelete, _ = strconv.ParseBool(autoDelete)
	}
	if exclusive := lookup("exclusive"); exclusive != "" {
		c.Exclusive, _ = strconv.ParseBool(exclusive)
	}
	if noWait := lookup("no_wait"); noWait != "" {
		c.NoWait, _ = strconv.ParseBool(noWait)
	}
	if tls := lookup("tls"); tls != "" {
		c.TLS, _ = strconv.ParseBool(tls)
	}
	if tlsInsecureSkipVerify := lookup("tls_insecure_skip_verify"); tlsInsecureSkipVerify != "" {
		c.TLSInsecureSkipVerify, _ = strconv.ParseBool(tlsInsecureSkipVerify)
	}
	if tlsCAFile := lookup("tls_ca_file"); tlsCAFile != "" {
		c.TLSCAFile = tlsCAFile
	}
	if tlsCertFile := lookup("tls_cert_file"); tlsCertFile != "" {
		c.TLSCertFile = tlsCertFile
	}
	if tlsKeyFile := lookup("tls_key_file"); tlsKeyFile != "" {
		c.TLSKeyFile = tlsKeyFile
	}
	if connectOnStartup := lookup("connect_on_startup"); connectOnStartup != "" {
		c.ConnectOnStartup, _ = strconv.ParseBool(connectOnStartup)
	}
	if queueType := lookup("queue_type"); queueType != "" {
		c.QueueType = queueType
	}
	if queueMode := lookup("queue_mode"); queueMode != "" {
		c.QueueMode = queueMode
	}
	if persistent := lookup("persistent"); persistent != "" {
		v, err := strconv.ParseBool(persistent)
		if err != nil {
			return fmt.Errorf("invalid persistent %q: %w", persistent, err)
		}
		c.Persistent = v
	}
	if expiration := lookup("expiration"); expiration != "" {
		v, err := strconv.Atoi(expiration)
		if err != nil {
			return fmt.Errorf("invalid expiration %q: %w", expiration, err)
		}
		c.Expiration = v
	}
	if contentType := lookup("content_type"); contentType != "" {
		c.ContentType = contentType
	}
	if messageType := lookup("message_type"); messageType != "" {
		c.MessageType = messageType
	}
	if priority := lookup("priority"); priority != "" {
		v, err := strconv.Atoi(priority)
		if err != nil {
			return fmt.Errorf("invalid priority %q: %w", priority, err)
		}
		c.Priority = v
	}
	if appID := lookup("app_id"); appID != "" {
		c.AppID = appID
	}
	if prefetch := lookup("prefetch_count"); prefetch != "" {
		v, err := strconv.Atoi(prefetch)
		if err != nil {
			return fmt.Errorf("invalid prefetch_count %q: %w", prefetch, err)
		}
		c.PrefetchCount = v
	}
	if decode := lookup("cloudevents_decode"); decode != "" {
		v, err := strconv.ParseBool(decode)
		if err != nil {
			return fmt.Errorf("invalid cloudevents_decode %q: %w", decode, err)
		}
		c.CloudEventsDecode = v
	}
	if backend := lookup("backend"); backend != "" {
		c.Backend = backend
	}
	if broker := lookup("broker"); broker != "" {
		c.MQTT.Broker = broker
	}
	if topic := lookup("topic"); topic != "" {
		c.MQTT.Topic = topic
	}
	if qos := lookup("qos"); qos != "" {
		v, err := strconv.Atoi(qos)
		if err != nil {
			return fmt.Errorf("invalid qos %q: %w", qos, err)
		}
		c.MQTT.QoS = v
	}
	if retain := lookup("retain"); retain != "" {
		v, err := strconv.ParseBool(retain)
		if err != nil {
			return fmt.Errorf("invalid retain %q: %w", retain, err)
		}
		c.MQTT.Retain = v
	}
	if clientID := lookup("client_id"); clientID != "" {
		c.MQTT.ClientID = clientID
	}
	if username := lookup("username"); username != "" {
		c.MQTT.Username = username
	}
	if password := lookup("password"); password != "" {
		c.MQTT.Password = password
	}
	if keepalive := lookup("keepalive"); keepalive != "" {
		v, err := strconv.Atoi(keepalive)
		if err != nil {
			return fmt.Errorf("invalid keepalive %q: %w", keepalive, err)
		}
		c.MQTT.Keepalive = v
	}
	if caCert := lookup("tls_ca_cert"); caCert != "" {
		c.MQTT.TLSCACert = caCert
	}
	if clientCert := lookup("tls_client_cert"); clientCert != "" {
		c.MQTT.TLSClientCert = clientCert
	}
	if clientKey := lookup("tls_client_key"); clientKey != "" {
		c.MQTT.TLSClientKey = clientKey
	}
	return nil
}
