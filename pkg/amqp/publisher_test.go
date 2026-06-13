package amqp

import (
	"testing"
	"time"

	"github.com/rossigee/fluent-bit-amqp-plugin/pkg/config"
)

func TestNewPublisher_NoConnection(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.URL = "amqp://invalid-host-that-does-not-exist:5672/"

	publisher, err := NewPublisher(cfg)
	if err != nil {
		t.Fatalf("Expected no error during NewPublisher, got: %v", err)
	}

	if publisher.connection != nil {
		t.Error("Expected connection to be nil (lazy connection)")
	}
	if publisher.channel != nil {
		t.Error("Expected channel to be nil (lazy connection)")
	}
}

func TestPublisher_CloseNeverConnected(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.URL = "amqp://invalid-host-that-does-not-exist:5672/"

	publisher, err := NewPublisher(cfg)
	if err != nil {
		t.Fatalf("Expected no error during NewPublisher, got: %v", err)
	}

	err = publisher.Close()
	if err != nil {
		t.Fatalf("Expected no error when closing never-connected publisher, got: %v", err)
	}

	if publisher.connection != nil {
		t.Error("Expected connection to be nil after close")
	}
	if publisher.channel != nil {
		t.Error("Expected channel to be nil after close")
	}
}

func TestPublisher_EnsureConnection_InvalidHost(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.URL = "amqp://invalid-host-that-does-not-exist:5672/"

	publisher, err := NewPublisher(cfg)
	if err != nil {
		t.Fatalf("Expected no error during NewPublisher, got: %v", err)
	}

	err = publisher.ensureConnection()
	if err == nil {
		t.Error("Expected error when connecting to invalid host")
	}
}

func TestPublishRecord_InvalidHost(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.URL = "amqp://invalid-host-that-does-not-exist:5672/"

	publisher, err := NewPublisher(cfg)
	if err != nil {
		t.Fatalf("Expected no error during NewPublisher, got: %v", err)
	}

	ts := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	record := map[string]interface{}{"message": "hello"}
	err = publisher.PublishRecord(ts, record, "test.tag")
	if err == nil {
		t.Error("Expected error when publishing to unreachable host")
	}
}

func TestNormalizeRecord_StringKeys(t *testing.T) {
	input := map[string]interface{}{"a": 1, "b": "two"}
	result := normalizeRecord(input)
	m, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map[string]interface{}, got %T", result)
	}
	if m["a"] != 1 || m["b"] != "two" {
		t.Errorf("unexpected map contents: %v", m)
	}
}

func TestNormalizeRecord_InterfaceKeys(t *testing.T) {
	input := map[interface{}]interface{}{
		"msg":    "hello",
		"level":  "info",
		"nested": map[interface{}]interface{}{"k": "v"},
	}
	result := normalizeRecord(input)
	m, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map[string]interface{}, got %T", result)
	}
	if m["msg"] != "hello" {
		t.Errorf("expected msg=hello, got %v", m)
	}
	nested, ok := m["nested"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected nested map[string]interface{}, got %T", m["nested"])
	}
	if nested["k"] != "v" {
		t.Errorf("expected nested k=v, got %v", nested)
	}
}

func TestNormalizeRecord_Slice(t *testing.T) {
	input := []interface{}{
		map[interface{}]interface{}{"x": 1},
		"plain",
	}
	result := normalizeRecord(input)
	s, ok := result.([]interface{})
	if !ok {
		t.Fatalf("expected []interface{}, got %T", result)
	}
	if len(s) != 2 {
		t.Fatalf("expected length 2, got %d", len(s))
	}
	m, ok := s[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected map[string]interface{} in slice, got %T", s[0])
	}
	if m["x"] != 1 {
		t.Errorf("expected x=1, got %v", m)
	}
}

func TestEncodeURLPassword(t *testing.T) {
	tests := []struct {
		name    string
		rawURL  string
		wantURL string
	}{
		{
			name:    "no password",
			rawURL:  "amqp://guest@localhost:5672/",
			wantURL: "amqp://guest@localhost:5672/",
		},
		{
			name:    "simple password",
			rawURL:  "amqp://guest:password@localhost:5672/",
			wantURL: "amqp://guest:password@localhost:5672/",
		},
		{
			name:    "password with plus",
			rawURL:  "amqp://user:QR+bllqd3Ikt@localhost:5672/",
			wantURL: "amqp://user:QR%2Bbllqd3Ikt@localhost:5672/",
		},
		{
			name:    "password with equals",
			rawURL:  "amqp://user:abc=123@localhost:5672/",
			wantURL: "amqp://user:abc%3D123@localhost:5672/",
		},
		{
			name:    "complex password like vault",
			rawURL:  "amqp://fluentbit:QR+bllqd3IktDyX9FolFT5+Aq+8Fdv9A@queues.bankrut.lan:5672/system",
			wantURL: "amqp://fluentbit:QR%2Bbllqd3IktDyX9FolFT5%2BAq%2B8Fdv9A@queues.bankrut.lan:5672/system",
		},
		{
			name:    "already encoded password - should not double encode",
			rawURL:  "amqps://fluentbit:Fz%2FZC7XqFnb%2F9uGZkjiprNqbR0HgHIjc@queues.bankrut.lan:5671/system",
			wantURL: "amqps://fluentbit:Fz%2FZC7XqFnb%2F9uGZkjiprNqbR0HgHIjc@queues.bankrut.lan:5671/system",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := encodeURLPassword(tt.rawURL)
			if err != nil {
				t.Errorf("encodeURLPassword() error = %v", err)
				return
			}
			if got != tt.wantURL {
				t.Errorf("encodeURLPassword() = %v, want %v", got, tt.wantURL)
			}
		})
	}
}