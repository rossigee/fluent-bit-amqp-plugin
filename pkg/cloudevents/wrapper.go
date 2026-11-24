package cloudevents

import (
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/google/uuid"
	"github.com/rossigee/fluent-bit-amqp-plugin/pkg/payload"
)

// WrapperConfig holds configuration for CloudEvent creation
type WrapperConfig struct {
	Source string
	Type   string
}

// Wrapper handles CloudEvent creation and marshaling
type Wrapper struct {
	config *WrapperConfig
}

// NewWrapper creates a new CloudEvent wrapper with the given configuration
func NewWrapper(config *WrapperConfig) *Wrapper {
	return &Wrapper{
		config: config,
	}
}

// normalizeRecord converts map[interface{}]interface{} to map[string]interface{}
// WrapRecord converts a Fluent Bit record to a CloudEvent
func (w *Wrapper) WrapRecord(timestamp time.Time, record interface{}, tag string) (*cloudevents.Event, error) {
	// Create CloudEvent
	ce := cloudevents.NewEvent()
	ce.SetID(uuid.New().String())
	ce.SetSource(w.config.Source)
	ce.SetType(w.config.Type)
	ce.SetTime(timestamp)
	ce.SetSpecVersion(cloudevents.VersionV1)

	// Normalize record to ensure JSON marshalable types
	normalized := payload.Normalize(record)

	// Set data from Fluent Bit record
	if err := ce.SetData(cloudevents.ApplicationJSON, normalized); err != nil {
		return nil, err
	}

	// Add Fluent Bit tag as extension if provided
	if tag != "" {
		ce.SetExtension("fluentbittag", tag)
	}

	return &ce, nil
}
