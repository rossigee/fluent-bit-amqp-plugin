package main

import (
	"C"
	"log"
	"os"
	"time"
	"unsafe"

	"github.com/fluent/fluent-bit-go/output"
	"github.com/rossigee/fluent-bit-amqp-plugin/pkg/amqp"
	"github.com/rossigee/fluent-bit-amqp-plugin/pkg/cloudevents"
	"github.com/rossigee/fluent-bit-amqp-plugin/pkg/config"
)

// Global plugin state
type PluginState struct {
	config    *config.AMQPConfig
	publisher *amqp.Publisher
	wrapper   *cloudevents.Wrapper
}

var pluginState *PluginState

//export FLBPluginRegister
func FLBPluginRegister(def unsafe.Pointer) int {
	return output.FLBPluginRegister(def, "amqp_cloudevents", "Send events to AMQP queue as CloudEvents")
}

//export FLBPluginInit
func FLBPluginInit(plugin unsafe.Pointer) int {
	// Initialize configuration with defaults
	cfg := config.DefaultConfig()

	// Load configuration from Fluent Bit
	cfg.LoadFromFluentBit(plugin)

	// Initialize AMQP publisher
	publisher, err := amqp.NewPublisher(cfg)
	if err != nil {
		log.Printf("Failed to initialize AMQP publisher: %v", err)
		return output.FLB_ERROR
	}

	// Initialize CloudEvent wrapper
	wrapperConfig := &cloudevents.WrapperConfig{
		Source: cfg.EventSource,
		Type:   cfg.EventType,
	}
	wrapper := cloudevents.NewWrapper(wrapperConfig)

	// Store plugin state
	pluginState = &PluginState{
		config:    cfg,
		publisher: publisher,
		wrapper:   wrapper,
	}

	log.Printf("AMQP CloudEvents plugin initialized - URL: %s, Queue: %s", cfg.URL, cfg.Queue)
	return output.FLB_OK
}

//export FLBPluginFlushCtx
func FLBPluginFlushCtx(ctx, data unsafe.Pointer, length C.int, tag *C.char) int {
	if pluginState == nil {
		log.Printf("Plugin not initialized")
		return output.FLB_ERROR
	}

	// Decode Fluent Bit records
	dec := output.NewDecoder(data, int(length))
	count := 0
	errors := 0

	tagStr := C.GoString(tag)

	for {
		ret, ts, record := output.GetRecord(dec)
		if ret != 0 {
			break
		}

		// Convert timestamp
		timestamp := time.Unix(ts.(output.FLBTime).Unix(), 0)

		// Create CloudEvent
		event, err := pluginState.wrapper.WrapRecord(timestamp, record, tagStr)
		if err != nil {
			log.Printf("Failed to wrap record as CloudEvent: %v", err)
			errors++
			continue
		}

		// Publish CloudEvent to AMQP
		if err := pluginState.publisher.PublishCloudEvent(event); err != nil {
			log.Printf("Failed to publish CloudEvent: %v", err)
			errors++
			continue
		}

		count++
	}

	log.Printf("Successfully published %d events to AMQP queue", count)

	if errors > 0 {
		log.Printf("Failed to publish %d events", errors)
		return output.FLB_RETRY
	}

	return output.FLB_OK
}

//export FLBPluginExit
func FLBPluginExit() int {
	if pluginState != nil {
		if pluginState.publisher != nil {
			if err := pluginState.publisher.Close(); err != nil {
				log.Printf("Error closing AMQP publisher: %v", err)
			}
		}
		log.Printf("AMQP CloudEvents plugin shutdown")
	}
	return output.FLB_OK
}

func main() {
	// This function is required but not called when used as a plugin
	log.Printf("Fluent Bit AMQP CloudEvents output plugin v%s", getVersion())
	os.Exit(0)
}

// getVersion returns the plugin version
func getVersion() string {
	// This would be set during build time
	return "0.1.0"
}
