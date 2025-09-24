package config

import (
	"strconv"
	"unsafe"

	"github.com/fluent/fluent-bit-go/output"
)

// AMQPConfig holds the configuration for the AMQP output plugin
type AMQPConfig struct {
	URL         string
	Exchange    string
	RoutingKey  string
	Queue       string
	EventSource string
	EventType   string
	Durable     bool
	AutoDelete  bool
	Exclusive   bool
	NoWait      bool
}

// DefaultConfig returns a new AMQPConfig with default values
func DefaultConfig() *AMQPConfig {
	return &AMQPConfig{
		URL:         "amqp://guest:guest@localhost:5672/",
		Exchange:    "",
		RoutingKey:  "fluent-bit-events",
		Queue:       "fluent-bit-events",
		EventSource: "fluent-bit",
		EventType:   "fluent-bit.log",
		Durable:     true,
		AutoDelete:  false,
		Exclusive:   false,
		NoWait:      false,
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
}
