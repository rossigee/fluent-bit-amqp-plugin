package config

import (
	"strings"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	tests := []struct {
		name string
		got  interface{}
		want interface{}
	}{
		{"URL", cfg.URL, "amqp://localhost:5672/"},
		{"Exchange", cfg.Exchange, ""},
		{"RoutingKey", cfg.RoutingKey, "fluent-bit-events"},
		{"Queue", cfg.Queue, "fluent-bit-events"},
		{"CloudEvents", cfg.CloudEvents, true},
		{"EventSource", cfg.EventSource, "fluent-bit"},
		{"EventType", cfg.EventType, "fluent-bit.log"},
		{"Durable", cfg.Durable, true},
		{"AutoDelete", cfg.AutoDelete, false},
		{"Exclusive", cfg.Exclusive, false},
		{"NoWait", cfg.NoWait, false},
		{"QueueType", cfg.QueueType, "classic"},
		{"QueueMode", cfg.QueueMode, "default"},
		{"Persistent", cfg.Persistent, true},
		{"Expiration", cfg.Expiration, 0},
		{"ContentType", cfg.ContentType, "application/json"},
		{"MessageType", cfg.MessageType, ""},
		{"Priority", cfg.Priority, 0},
		{"AppID", cfg.AppID, "fluent-bit"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("got %v, want %v", tt.got, tt.want)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(c *AMQPConfig)
		wantErr string
	}{
		{name: "defaults are valid", modify: func(c *AMQPConfig) {}},
		{name: "classic lazy is valid", modify: func(c *AMQPConfig) { c.QueueMode = QueueModeLazy }},
		{name: "quorum durable is valid", modify: func(c *AMQPConfig) { c.QueueType = QueueTypeQuorum }},
		{name: "stream durable is valid", modify: func(c *AMQPConfig) { c.QueueType = QueueTypeStream }},
		{name: "empty queue type treated as classic", modify: func(c *AMQPConfig) { c.QueueType = "" }},
		{name: "priority at upper bound", modify: func(c *AMQPConfig) { c.Priority = 255 }},
		{name: "expiration set", modify: func(c *AMQPConfig) { c.Expiration = 60000 }},
		{
			name:    "unknown queue type",
			modify:  func(c *AMQPConfig) { c.QueueType = "fast" },
			wantErr: `invalid queue_type "fast"`,
		},
		{
			name:    "unknown queue mode",
			modify:  func(c *AMQPConfig) { c.QueueMode = "eager" },
			wantErr: `invalid queue_mode "eager"`,
		},
		{
			name:    "lazy mode on quorum queue",
			modify:  func(c *AMQPConfig) { c.QueueType = QueueTypeQuorum; c.QueueMode = QueueModeLazy },
			wantErr: "only applies to classic queues",
		},
		{
			name:    "quorum requires durable",
			modify:  func(c *AMQPConfig) { c.QueueType = QueueTypeQuorum; c.Durable = false },
			wantErr: "requires durable=true",
		},
		{
			name:    "stream requires durable",
			modify:  func(c *AMQPConfig) { c.QueueType = QueueTypeStream; c.Durable = false },
			wantErr: "requires durable=true",
		},
		{
			name:    "quorum rejects exclusive",
			modify:  func(c *AMQPConfig) { c.QueueType = QueueTypeQuorum; c.Exclusive = true },
			wantErr: "does not support exclusive=true",
		},
		{
			name:    "stream rejects auto_delete",
			modify:  func(c *AMQPConfig) { c.QueueType = QueueTypeStream; c.AutoDelete = true },
			wantErr: "does not support auto_delete=true",
		},
		{
			name:    "negative expiration",
			modify:  func(c *AMQPConfig) { c.Expiration = -1 },
			wantErr: "invalid expiration -1",
		},
		{
			name:    "priority above 255",
			modify:  func(c *AMQPConfig) { c.Priority = 256 },
			wantErr: "invalid priority 256",
		},
		{
			name:    "negative priority",
			modify:  func(c *AMQPConfig) { c.Priority = -1 },
			wantErr: "invalid priority -1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			tt.modify(cfg)
			err := cfg.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Validate() expected error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("Validate() error = %q, want it to contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func lookupFrom(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func TestLoadFrom_NewKeys(t *testing.T) {
	cfg := DefaultConfig()
	err := cfg.loadFrom(lookupFrom(map[string]string{
		"queue_type":   "quorum",
		"queue_mode":   "lazy",
		"persistent":   "false",
		"expiration":   "60000",
		"content_type": "application/cloudevents+json",
		"message_type": "log",
		"priority":     "5",
		"app_id":       "edge-01",
	}))
	if err != nil {
		t.Fatalf("loadFrom() error = %v", err)
	}

	if cfg.QueueType != QueueTypeQuorum {
		t.Errorf("QueueType = %q, want quorum", cfg.QueueType)
	}
	if cfg.QueueMode != QueueModeLazy {
		t.Errorf("QueueMode = %q, want lazy", cfg.QueueMode)
	}
	if cfg.Persistent {
		t.Error("Persistent = true, want false")
	}
	if cfg.Expiration != 60000 {
		t.Errorf("Expiration = %d, want 60000", cfg.Expiration)
	}
	if cfg.ContentType != "application/cloudevents+json" {
		t.Errorf("ContentType = %q", cfg.ContentType)
	}
	if cfg.MessageType != "log" {
		t.Errorf("MessageType = %q, want log", cfg.MessageType)
	}
	if cfg.Priority != 5 {
		t.Errorf("Priority = %d, want 5", cfg.Priority)
	}
	if cfg.AppID != "edge-01" {
		t.Errorf("AppID = %q, want edge-01", cfg.AppID)
	}
}

func TestLoadFrom_UnsetKeysKeepDefaults(t *testing.T) {
	cfg := DefaultConfig()
	if err := cfg.loadFrom(lookupFrom(map[string]string{})); err != nil {
		t.Fatalf("loadFrom() error = %v", err)
	}

	want := DefaultConfig()
	if cfg.QueueType != want.QueueType || cfg.QueueMode != want.QueueMode ||
		cfg.Persistent != want.Persistent || cfg.Expiration != want.Expiration ||
		cfg.ContentType != want.ContentType || cfg.MessageType != want.MessageType ||
		cfg.Priority != want.Priority || cfg.AppID != want.AppID {
		t.Errorf("unset keys changed defaults: got %+v, want %+v", cfg, want)
	}
}

func TestLoadFrom_InvalidValues(t *testing.T) {
	tests := []struct {
		name    string
		values  map[string]string
		wantErr string
	}{
		{name: "non-boolean persistent", values: map[string]string{"persistent": "yes"}, wantErr: `invalid persistent "yes"`},
		{name: "non-numeric expiration", values: map[string]string{"expiration": "1m"}, wantErr: `invalid expiration "1m"`},
		{name: "non-numeric priority", values: map[string]string{"priority": "high"}, wantErr: `invalid priority "high"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			err := cfg.loadFrom(lookupFrom(tt.values))
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("loadFrom() error = %v, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}
