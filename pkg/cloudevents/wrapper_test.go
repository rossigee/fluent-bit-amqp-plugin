package cloudevents

import (
	"encoding/json"
	"testing"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
)

func newTestWrapper() *Wrapper {
	return NewWrapper(&WrapperConfig{
		Source: "test-source",
		Type:   "test.type",
	})
}

func TestNewWrapper(t *testing.T) {
	w := newTestWrapper()
	if w == nil {
		t.Fatal("expected non-nil Wrapper")
	}
	if w.config.Source != "test-source" {
		t.Errorf("got source %q, want %q", w.config.Source, "test-source")
	}
	if w.config.Type != "test.type" {
		t.Errorf("got type %q, want %q", w.config.Type, "test.type")
	}
}

func TestWrapRecord_Fields(t *testing.T) {
	w := newTestWrapper()
	ts := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	record := map[string]interface{}{"message": "hello"}

	event, err := w.WrapRecord(ts, record, "my.tag")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if event.Source() != "test-source" {
		t.Errorf("got source %q, want %q", event.Source(), "test-source")
	}
	if event.Type() != "test.type" {
		t.Errorf("got type %q, want %q", event.Type(), "test.type")
	}
	if event.SpecVersion() != cloudevents.VersionV1 {
		t.Errorf("got spec version %q, want %q", event.SpecVersion(), cloudevents.VersionV1)
	}
	if event.Time() != ts {
		t.Errorf("got time %v, want %v", event.Time(), ts)
	}
	if event.ID() == "" {
		t.Error("expected non-empty event ID")
	}
}

func TestWrapRecord_TagExtension(t *testing.T) {
	w := newTestWrapper()
	ts := time.Now()
	record := map[string]interface{}{"k": "v"}

	t.Run("tag set", func(t *testing.T) {
		event, err := w.WrapRecord(ts, record, "some.tag")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		ext := event.Extensions()
		if ext == nil {
			t.Fatal("expected extensions to be set")
		}
		if ext["fluentbittag"] != "some.tag" {
			t.Errorf("got fluentbittag %v, want %q", ext["fluentbittag"], "some.tag")
		}
	})

	t.Run("tag empty", func(t *testing.T) {
		event, err := w.WrapRecord(ts, record, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		ext := event.Extensions()
		if _, ok := ext["fluentbittag"]; ok {
			t.Error("expected fluentbittag extension to be absent when tag is empty")
		}
	})
}

func TestWrapRecord_UniqueIDs(t *testing.T) {
	w := newTestWrapper()
	ts := time.Now()
	record := map[string]interface{}{}

	e1, err := w.WrapRecord(ts, record, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	e2, err := w.WrapRecord(ts, record, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if e1.ID() == e2.ID() {
		t.Error("expected unique IDs per event, got duplicates")
	}
}

func TestWrapRecord_DataSerializable(t *testing.T) {
	w := newTestWrapper()
	ts := time.Now()
	record := map[string]interface{}{"level": "info", "msg": "test"}

	event, err := w.WrapRecord(ts, record, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the event can be marshaled to JSON (as the publisher does)
	_, err = json.Marshal(event)
	if err != nil {
		t.Errorf("event not JSON-serializable: %v", err)
	}
}
