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
	wrapper   *cloudevents.Wrapper // nil when CloudEvents is disabled
}

var pluginState *PluginState

//export FLBPluginRegister
func FLBPluginRegister(def unsafe.Pointer) int {
	return output.FLBPluginRegister(def, "amqp", "Send log records to AMQP queue (plain JSON or CloudEvents)")
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

	state := &PluginState{
		config:    cfg,
		publisher: publisher,
	}

	// Initialize CloudEvent wrapper only when enabled
	if cfg.CloudEvents {
		state.wrapper = cloudevents.NewWrapper(&cloudevents.WrapperConfig{
			Source: cfg.EventSource,
			Type:   cfg.EventType,
		})
		log.Printf("AMQP plugin initialized (CloudEvents) - URL: %s, Queue: %s, Source: %s, Type: %s",
			cfg.URL, cfg.Queue, cfg.EventSource, cfg.EventType)
	} else {
		log.Printf("AMQP plugin initialized (plain JSON) - URL: %s, Queue: %s", cfg.URL, cfg.Queue)
	}

	pluginState = state
	return output.FLB_OK
}

//export FLBPluginFlushCtx
func FLBPluginFlushCtx(ctx, data unsafe.Pointer, length C.int, tag *C.char) int {
	if pluginState == nil {
		log.Printf("Plugin not initialized")
		return output.FLB_ERROR
	}

	dec := output.NewDecoder(data, int(length))
	count := 0
	errors := 0
	tagStr := C.GoString(tag)

	for {
		ret, ts, record := output.GetRecord(dec)
		if ret != 0 {
			break
		}

		timestamp := time.Unix(ts.(output.FLBTime).Unix(), 0)

		var err error
		if pluginState.wrapper != nil {
			event, wrapErr := pluginState.wrapper.WrapRecord(timestamp, record, tagStr)
			if wrapErr != nil {
				log.Printf("Failed to wrap record as CloudEvent: %v", wrapErr)
				errors++
				continue
			}
			err = pluginState.publisher.PublishCloudEvent(event)
		} else {
			err = pluginState.publisher.PublishRecord(timestamp, record, tagStr)
		}

		if err != nil {
			log.Printf("Failed to publish record: %v", err)
			errors++
			continue
		}

		count++
	}

	log.Printf("Successfully published %d records to AMQP queue", count)

	if errors > 0 {
		log.Printf("Failed to publish %d records", errors)
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
		log.Printf("AMQP plugin shutdown")
	}
	return output.FLB_OK
}

var version = "dev"

func main() {
	log.Printf("Fluent Bit AMQP output plugin v%s", version)
	os.Exit(0)
}
