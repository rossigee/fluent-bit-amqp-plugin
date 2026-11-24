package main

import (
	"C"
	"net/url"
	"os"
	"strings"
	"time"
	"unsafe"

	cesdk "github.com/cloudevents/sdk-go/v2"
	"github.com/fluent/fluent-bit-go/output"
	"github.com/rossigee/fluent-bit-amqp-plugin/pkg/amqp"
	"github.com/rossigee/fluent-bit-amqp-plugin/pkg/cloudevents"
	"github.com/rossigee/fluent-bit-amqp-plugin/pkg/config"
	"github.com/rossigee/fluent-bit-amqp-plugin/pkg/mqtt"
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

// publisher is the transport-independent interface implemented by the AMQP and MQTT publishers
type publisher interface {
	SetLogger(logger *zap.SugaredLogger)
	EnsureConnection() error
	PublishCloudEvent(event *cesdk.Event) error
	PublishRecord(timestamp time.Time, record interface{}, tag string) error
	Close() error
}

// newPublisher returns the publisher for the configured backend
func newPublisher(cfg *config.AMQPConfig) (publisher, error) {
	if cfg.Backend == config.BackendMQTT {
		return mqtt.NewPublisher(cfg)
	}
	return amqp.NewPublisher(cfg)
}

// destination describes where the plugin publishes, for log messages
func destination(cfg *config.AMQPConfig) string {
	if cfg.Backend == config.BackendMQTT {
		return cfg.MQTT.Broker + " topic=" + cfg.MQTT.Topic
	}
	exchange := cfg.Exchange
	if exchange == "" {
		exchange = "(default)"
	}
	return maskPassword(cfg.URL) + " exchange=" + exchange + " routing_key=" + cfg.RoutingKey
}

// Global plugin state
type PluginState struct {
	config    *config.AMQPConfig
	publisher publisher
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
	return output.FLBPluginRegisterWithOptions(def,
		output.WithName("amqp"),
		output.WithDescription("Send log records to an AMQP queue or MQTT topic (plain JSON or CloudEvents)"),
	)
}

//export FLBPluginInit
func FLBPluginInit(plugin unsafe.Pointer) int {
	// Initialize configuration with defaults
	cfg := config.DefaultConfig()

	// Load configuration from Fluent Bit
	if err := cfg.LoadFromFluentBit(plugin); err != nil {
		logger.Errorf("Failed to load configuration: %v", err)
		return output.FLB_ERROR
	}

	pub, err := newPublisher(cfg)
	if err != nil {
		logger.Errorf("Failed to initialize %s publisher: %v", cfg.Backend, err)
		return output.FLB_ERROR
	}
	pub.SetLogger(logger)

	state := &PluginState{
		config:    cfg,
		publisher: pub,
	}

	// Eagerly connect on startup if configured
	if cfg.ConnectOnStartup {
		if err := pub.EnsureConnection(); err != nil {
			logger.Errorw("Failed to establish connection on startup", "backend", cfg.Backend, "error", err)
			return output.FLB_ERROR
		}
		logger.Infow("Connection established on startup", "backend", cfg.Backend, "destination", destination(cfg))
	}

	// Initialize CloudEvent wrapper only when enabled
	mode := "plain JSON"
	if cfg.CloudEvents {
		state.wrapper = cloudevents.NewWrapper(&cloudevents.WrapperConfig{
			Source: cfg.EventSource,
			Type:   cfg.EventType,
		})
		mode = "CloudEvents"
	}
	logger.Infow("Output plugin initialized",
		"backend", cfg.Backend,
		"mode", mode,
		"destination", destination(cfg))

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

	if count > 0 {
		logger.Infow("Successfully published records", "backend", pluginState.config.Backend, "count", count, "destination", destination(pluginState.config))
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
				logger.Errorw("Error closing publisher", "error", err)
			}
		}
		logger.Info("Output plugin shutdown")
	}
	return output.FLB_OK
}

var version = "dev"

func main() {
	logger = initLogger(false)
	logger.Infow("Fluent Bit AMQP output plugin", "version", version)
	os.Exit(0)
}
