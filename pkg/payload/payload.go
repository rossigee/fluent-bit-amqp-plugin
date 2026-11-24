// Package payload builds the message bodies shared by every output backend.
package payload

import (
	"encoding/json"
	"fmt"
	"time"
)

// Normalize converts map[interface{}]interface{} to map[string]interface{}
// recursively, which is required for JSON marshaling support. The Fluent Bit decoder
// returns string values as []byte, which encoding/json would emit as base64, so those
// are converted back to strings.
func Normalize(v interface{}) interface{} {
	switch v := v.(type) {
	case map[interface{}]interface{}:
		result := make(map[string]interface{}, len(v))
		for key, value := range v {
			strKey := fmt.Sprintf("%v", key)
			result[strKey] = Normalize(value)
		}
		return result
	case []interface{}:
		result := make([]interface{}, len(v))
		for i, item := range v {
			result[i] = Normalize(item)
		}
		return result
	case []byte:
		return string(v)
	default:
		return v
	}
}

// Record returns the plain JSON body for a Fluent Bit record. The record's fields
// are kept at the top level, with "@timestamp" and, when tag is set, "tag" added.
// A record that is not a map is wrapped as {"data": <record>}.
func Record(timestamp time.Time, record interface{}, tag string) ([]byte, error) {
	normalized, ok := Normalize(record).(map[string]interface{})
	if !ok {
		normalized = map[string]interface{}{"data": Normalize(record)}
	}

	// Merge metadata into a shallow copy so we don't mutate the original.
	payload := make(map[string]interface{}, len(normalized)+2)
	for k, v := range normalized {
		payload[k] = v
	}
	payload["@timestamp"] = timestamp.UTC().Format(time.RFC3339)
	if tag != "" {
		payload["tag"] = tag
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal record: %w", err)
	}
	return body, nil
}
