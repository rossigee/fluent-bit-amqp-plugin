package config

import (
	"strings"
	"testing"
)

func TestLoadFrom_MQTTKeys(t *testing.T) {
	cfg := DefaultConfig()
	err := cfg.LoadFromLookup(lookupFrom(map[string]string{
		"backend":         "mqtt",
		"broker":          "ssl://mqtt.example.com:8883",
		"topic":           "logs/edge",
		"qos":             "2",
		"retain":          "true",
		"client_id":       "edge-01",
		"username":        "edge",
		"password":        "secret",
		"keepalive":       "45",
		"tls_ca_cert":     "/etc/fluent-bit/ca.pem",
		"tls_client_cert": "/etc/fluent-bit/client.pem",
		"tls_client_key":  "/etc/fluent-bit/client.key",
	}))
	if err != nil {
		t.Fatalf("LoadFromLookup() error = %v", err)
	}

	if cfg.Backend != BackendMQTT {
		t.Errorf("Backend = %q, want mqtt", cfg.Backend)
	}
	m := cfg.MQTT
	if m.Broker != "ssl://mqtt.example.com:8883" || m.Topic != "logs/edge" || m.QoS != 2 || !m.Retain {
		t.Errorf("unexpected MQTT settings: %+v", m)
	}
	if m.ClientID != "edge-01" || m.Username != "edge" || m.Password != "secret" || m.Keepalive != 45 {
		t.Errorf("unexpected MQTT identity settings: %+v", m)
	}
	if m.TLSCACert != "/etc/fluent-bit/ca.pem" || m.TLSClientCert != "/etc/fluent-bit/client.pem" || m.TLSClientKey != "/etc/fluent-bit/client.key" {
		t.Errorf("unexpected MQTT TLS settings: %+v", m)
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() error = %v", err)
	}
}

func TestLoadFrom_MQTTInvalidValues(t *testing.T) {
	for _, tt := range []struct {
		key, value string
	}{
		{"qos", "high"},
		{"retain", "maybe"},
		{"keepalive", "soon"},
	} {
		t.Run(tt.key, func(t *testing.T) {
			cfg := DefaultConfig()
			if err := cfg.LoadFromLookup(lookupFrom(map[string]string{tt.key: tt.value})); err == nil {
				t.Errorf("expected error for %s=%q", tt.key, tt.value)
			}
		})
	}
}

func TestDefaultConfig_BackendIsAMQP(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Backend != BackendAMQP {
		t.Errorf("Backend = %q, want amqp", cfg.Backend)
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("default config should be valid: %v", err)
	}
}

func TestValidate_MQTT(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(c *AMQPConfig)
		wantErr string
	}{
		{
			name:   "defaults are valid",
			mutate: func(c *AMQPConfig) {},
		},
		{
			name:    "unknown backend",
			mutate:  func(c *AMQPConfig) { c.Backend = "kafka" },
			wantErr: "invalid backend",
		},
		{
			name:    "broker scheme must be tcp, ssl, ws or wss",
			mutate:  func(c *AMQPConfig) { c.MQTT.Broker = "http://broker:1883" },
			wantErr: "scheme must be",
		},
		{
			name:    "broker needs a host",
			mutate:  func(c *AMQPConfig) { c.MQTT.Broker = "tcp://" },
			wantErr: "missing host",
		},
		{
			name:    "topic is required",
			mutate:  func(c *AMQPConfig) { c.MQTT.Topic = "" },
			wantErr: "topic is required",
		},
		{
			name:    "qos above 2 is rejected",
			mutate:  func(c *AMQPConfig) { c.MQTT.QoS = 3 },
			wantErr: "invalid qos",
		},
		{
			name:    "qos below 0 is rejected",
			mutate:  func(c *AMQPConfig) { c.MQTT.QoS = -1 },
			wantErr: "invalid qos",
		},
		{
			name:    "keepalive must be positive",
			mutate:  func(c *AMQPConfig) { c.MQTT.Keepalive = 0 },
			wantErr: "invalid keepalive",
		},
		{
			name:    "client cert needs a key",
			mutate:  func(c *AMQPConfig) { c.MQTT.TLSClientCert = "/c.pem" },
			wantErr: "must be set together",
		},
		{
			name: "TLS files need an ssl broker",
			mutate: func(c *AMQPConfig) {
				c.MQTT.TLSCACert = "/ca.pem"
			},
			wantErr: "need an ssl:// or wss:// broker",
		},
		{
			name: "TLS files are accepted with an ssl broker",
			mutate: func(c *AMQPConfig) {
				c.MQTT.Broker = "ssl://broker:8883"
				c.MQTT.TLSCACert = "/ca.pem"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Backend = BackendMQTT
			tt.mutate(cfg)
			err := cfg.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Validate() error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestValidate_MQTTIgnoresAMQPQueueRules(t *testing.T) {
	// A queue type that is invalid for AMQP must not affect the MQTT backend.
	cfg := DefaultConfig()
	cfg.Backend = BackendMQTT
	cfg.QueueType = "not-a-queue-type"
	if err := cfg.Validate(); err != nil {
		t.Errorf("MQTT validation should ignore AMQP queue settings: %v", err)
	}
}
