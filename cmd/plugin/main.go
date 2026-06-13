package main

import (
	"C"
	"net/url"
	"os"
	"strings"
	"time"
	"unsafe"

	"github.com/fluent/fluent-bit-go/output"
	"github.com/rossigee/fluent-bit-amqp-plugin/pkg/amqp"
	"github.com/rossigee/fluent-bit-amqp-plugin/pkg/cloudevents"
	"github.com/rossigee/fluent-bit-amqp-plugin/pkg/config"
	"go.uber.org/zap"
)

func maskPassword(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.User == nil {
		return rawURL
	}
	_, hasPassword := u.User.Password()
	if !hasPassword {
		return rawURL
	}
	password, _ := u.User.Password()
	return strings.Replace(rawURL, password, "****", 1)
}

// Global plugin state
type PluginState struct {
	config    *config.AMQPConfig
	publisher *amqp.Publisher
	wrapper   *cloudevents.Wrapper // nil when CloudEvents is disabled
}

var pluginState *PluginState
var logger *zap.SugaredLogger

func initLogger(debug bool) *zap.SugaredLogger {
	var zapLogger *zap.Logger
	var err error
	if debug {
		zapLogger, err = zap.NewDevelopment()
	} else {
		zapLogger, err = zap.NewProduction()
	}
	if err != nil {
		return zap.NewNop().Sugar()
	}
	return zapLogger.Sugar()
}

//export FLBPluginRegister
func FLBPluginRegister(def unsafe.Pointer) int {
	logger = initLogger(false)
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
		logger.Errorf("Failed to initialize AMQP publisher: %v", err)
		return output.FLB_ERROR
	}
	publisher.SetLogger(logger)

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
		logger.Infow("AMQP plugin initialized",
			"mode", "CloudEvents",
			"url", maskPassword(cfg.URL),
			"queue", cfg.Queue,
			"source", cfg.EventSource,
			"type", cfg.EventType)
	} else {
		logger.Infow("AMQP plugin initialized",
			"mode", "plain JSON",
			"url", maskPassword(cfg.URL),
			"queue", cfg.Queue)
	}

	pluginState = state
	return output.FLB_OK
}

//export FLBPluginFlushCtx
func FLBPluginFlushCtx(ctx, data unsafe.Pointer, length C.int, tag *C.char) int {
	if pluginState == nil {
		logger.Error("Plugin not initialized")
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
				logger.Errorw("Failed to wrap record as CloudEvent", "error", wrapErr)
				errors++
				continue
			}
			err = pluginState.publisher.PublishCloudEvent(event)
		} else {
			err = pluginState.publisher.PublishRecord(timestamp, record, tagStr)
		}

		if err != nil {
			logger.Errorw("Failed to publish record", "error", err)
			errors++
			continue
		}

		count++
	}

	logger.Debugw("Published records", "count", count, "errors", errors)

	if errors > 0 {
		logger.Errorw("Publish completed with errors", "errors", errors)
		return output.FLB_RETRY
	}

	return output.FLB_OK
}

//export FLBPluginExit
func FLBPluginExit() int {
	if pluginState != nil {
		if pluginState.publisher != nil {
			if err := pluginState.publisher.Close(); err != nil {
				logger.Errorw("Error closing AMQP publisher", "error", err)
			}
		}
		logger.Info("AMQP plugin shutdown")
	}
	return output.FLB_OK
}

var version = "dev"

func main() {
	logger = initLogger(false)
	logger.Infow("Fluent Bit AMQP output plugin", "version", version)
	os.Exit(0)
}
