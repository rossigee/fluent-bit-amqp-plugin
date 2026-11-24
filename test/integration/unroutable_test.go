//go:build integration

package integration

import (
	"strings"
	"testing"
	"time"

	"github.com/rossigee/fluent-bit-amqp-plugin/pkg/amqp"
	"github.com/rossigee/fluent-bit-amqp-plugin/pkg/config"
)

// A publish to an exchange with no matching binding must fail, not be logged as
// delivered. Before publisher confirms and the mandatory flag were added, the
// broker discarded such a message and the plugin reported success.
func TestPublishUnroutableReturnsError(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.URL = rabbitmqURL(t)
	cfg.Exchange = "amq.direct"
	cfg.RoutingKey = "no-binding-" + time.Now().Format("150405.000000000")

	pub, err := amqp.NewPublisher(cfg)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer func() {
		if err := pub.Close(); err != nil {
			t.Logf("Close: %v", err)
		}
	}()

	err = pub.PublishRecord(time.Now(), map[string]interface{}{"msg": "unroutable"}, "test.unroutable")
	if err == nil {
		t.Fatal("PublishRecord to an unbound routing key returned nil, want an unroutable error")
	}
	if !strings.Contains(err.Error(), "unroutable") {
		t.Fatalf("error = %q, want it to mention unroutable", err)
	}
}
