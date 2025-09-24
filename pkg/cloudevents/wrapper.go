package cloudevents

import (
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/google/uuid"
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

// WrapRecord converts a Fluent Bit record to a CloudEvent
func (w *Wrapper) WrapRecord(timestamp time.Time, record interface{}, tag string) (*cloudevents.Event, error) {
	// Create CloudEvent
	ce := cloudevents.NewEvent()
	ce.SetID(uuid.New().String())
	ce.SetSource(w.config.Source)
	ce.SetType(w.config.Type)
	ce.SetTime(timestamp)
	ce.SetSpecVersion(cloudevents.VersionV1)

	// Set data from Fluent Bit record
	if err := ce.SetData(cloudevents.ApplicationJSON, record); err != nil {
		return nil, err
	}

	// Add Fluent Bit tag as extension if provided
	if tag != "" {
		ce.SetExtension("fluentbittag", tag)
	}

	return &ce, nil
}
