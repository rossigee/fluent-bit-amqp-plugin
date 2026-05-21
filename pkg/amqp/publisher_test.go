package amqp

import (
	"testing"

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
