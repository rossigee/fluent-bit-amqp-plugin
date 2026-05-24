package config

import (
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	tests := []struct {
		name string
		got  interface{}
		want interface{}
	}{
		{"URL", cfg.URL, "amqp://guest:guest@localhost:5672/"},
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("got %v, want %v", tt.got, tt.want)
			}
		})
	}
}
