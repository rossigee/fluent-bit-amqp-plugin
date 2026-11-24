package main

import (
	"strings"
	"testing"

	"github.com/rossigee/fluent-bit-amqp-plugin/pkg/config"
)

func TestDestinationNamesExchangeAndRoutingKey(t *testing.T) {
	cfg := &config.AMQPConfig{
		URL:        "amqps://user:secret@queues.example.lan:5671/system",
		Exchange:   "golder-cloudevents",
		RoutingKey: "alerts.golder.security",
		Queue:      "fluent-bit-events",
	}
	got := destination(cfg)
	want := "amqps://user:****@queues.example.lan:5671/system exchange=golder-cloudevents routing_key=alerts.golder.security"
	if got != want {
		t.Fatalf("destination() = %q, want %q", got, want)
	}
	if strings.Contains(got, "secret") {
		t.Fatalf("destination() leaked the password: %q", got)
	}
}

func TestDestinationShowsDefaultExchange(t *testing.T) {
	cfg := &config.AMQPConfig{
		URL:        "amqps://user:pw@queues.example.lan:5671/system",
		RoutingKey: "fluent-bit-events",
	}
	got := destination(cfg)
	if !strings.Contains(got, "exchange=(default) routing_key=fluent-bit-events") {
		t.Fatalf("destination() = %q, want default exchange named", got)
	}
}
