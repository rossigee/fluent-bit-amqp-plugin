package config

import (
	"strconv"
	"unsafe"

	"github.com/fluent/fluent-bit-go/output"
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
}

// DefaultConfig returns a new AMQPConfig with default values
func DefaultConfig() *AMQPConfig {
	return &AMQPConfig{
		URL:                   "amqp://guest:guest@localhost:5672/",
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
	}
}

// LoadFromFluentBit loads configuration from Fluent Bit plugin context
func (c *AMQPConfig) LoadFromFluentBit(plugin unsafe.Pointer) {
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
}
