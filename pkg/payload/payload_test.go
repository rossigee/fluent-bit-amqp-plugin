package payload

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func TestNormalize_ConvertsBytesAndMaps(t *testing.T) {
	in := map[interface{}]interface{}{
		"message": []byte("hello"),
		"nested":  map[interface{}]interface{}{"k": []byte("v")},
		"list":    []interface{}{[]byte("a"), 2},
	}
	got := Normalize(in)
	want := map[string]interface{}{
		"message": "hello",
		"nested":  map[string]interface{}{"k": "v"},
		"list":    []interface{}{"a", 2},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Normalize() = %#v, want %#v", got, want)
	}
}

func TestRecord_AddsTimestampAndTag(t *testing.T) {
	ts := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	body, err := Record(ts, map[string]interface{}{"message": "hi"}, "app.logs")
	if err != nil {
		t.Fatalf("Record() error = %v", err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("body is not JSON: %v", err)
	}
	if got["message"] != "hi" || got["@timestamp"] != "2024-06-01T12:00:00Z" || got["tag"] != "app.logs" {
		t.Errorf("unexpected body: %v", got)
	}
}

func TestRecord_OmitsEmptyTag(t *testing.T) {
	body, err := Record(time.Unix(0, 0), map[string]interface{}{"message": "hi"}, "")
	if err != nil {
		t.Fatalf("Record() error = %v", err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("body is not JSON: %v", err)
	}
	if _, ok := got["tag"]; ok {
		t.Errorf("tag should be omitted when empty: %v", got)
	}
}

func TestRecord_WrapsNonMapRecords(t *testing.T) {
	body, err := Record(time.Unix(0, 0), []interface{}{1, 2}, "t")
	if err != nil {
		t.Fatalf("Record() error = %v", err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("body is not JSON: %v", err)
	}
	if !reflect.DeepEqual(got["data"], []interface{}{float64(1), float64(2)}) {
		t.Errorf("expected data wrapper, got %v", got)
	}
}

func TestRecord_DoesNotMutateInput(t *testing.T) {
	in := map[string]interface{}{"message": "hi"}
	if _, err := Record(time.Unix(0, 0), in, "tag"); err != nil {
		t.Fatalf("Record() error = %v", err)
	}
	if len(in) != 1 {
		t.Errorf("input record was modified: %v", in)
	}
}
