package amqp

import (
	"reflect"
	"testing"

	"github.com/rossigee/fluent-bit-amqp-plugin/pkg/config"
)

func TestRecordFromBody(t *testing.T) {
	tests := []struct {
		name   string
		body   string
		decode bool
		want   map[string]interface{}
	}{
		{
			name: "plain JSON object is used as the record",
			body: `{"message":"hello","level":"info"}`,
			want: map[string]interface{}{"message": "hello", "level": "info"},
		},
		{
			name: "non-JSON body is wrapped as message",
			body: "plain text",
			want: map[string]interface{}{"message": "plain text"},
		},
		{
			name: "JSON array is not an object and is wrapped as message",
			body: `[1,2,3]`,
			want: map[string]interface{}{"message": "[1,2,3]"},
		},
		{
			name:   "CloudEvent is left intact when decoding is off",
			body:   `{"specversion":"1.0","type":"t","data":{"message":"inner"}}`,
			decode: false,
			want:   map[string]interface{}{"specversion": "1.0", "type": "t", "data": map[string]interface{}{"message": "inner"}},
		},
		{
			name:   "CloudEvent object data becomes the record when decoding",
			body:   `{"specversion":"1.0","type":"t","data":{"message":"inner"}}`,
			decode: true,
			want:   map[string]interface{}{"message": "inner"},
		},
		{
			name:   "CloudEvent scalar data is wrapped as data when decoding",
			body:   `{"specversion":"1.0","type":"t","data":"just a string"}`,
			decode: true,
			want:   map[string]interface{}{"data": "just a string"},
		},
		{
			name:   "non-CloudEvent object is passed through when decoding",
			body:   `{"plain":"not-an-event"}`,
			decode: true,
			want:   map[string]interface{}{"plain": "not-an-event"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RecordFromBody([]byte(tt.body), tt.decode)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("RecordFromBody() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewConsumer_RequiresQueue(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Queue = ""
	if _, err := NewConsumer(cfg); err == nil {
		t.Fatal("expected NewConsumer to reject an empty queue")
	}
}

func TestNewConsumer_RejectsInvalidPrefetch(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.PrefetchCount = 0
	if _, err := NewConsumer(cfg); err == nil {
		t.Fatal("expected NewConsumer to reject prefetch_count 0")
	}
}

func TestNewConsumer_BuffersPrefetchCount(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.PrefetchCount = 3
	c, err := NewConsumer(cfg)
	if err != nil {
		t.Fatalf("NewConsumer() error = %v", err)
	}
	if got := cap(c.buffer); got != 3 {
		t.Errorf("buffer capacity = %d, want 3", got)
	}
	if got := c.Next(10); len(got) != 0 {
		t.Errorf("Next() on an empty buffer returned %d deliveries", len(got))
	}
}
