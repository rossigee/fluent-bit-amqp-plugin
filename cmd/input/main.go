package main

import (
	"C"
	"context"
	"time"
	"unsafe"

	"github.com/fluent/fluent-bit-go/input"
	amqpbroker "github.com/rabbitmq/amqp091-go"
	"github.com/rossigee/fluent-bit-amqp-plugin/pkg/amqp"
	"github.com/rossigee/fluent-bit-amqp-plugin/pkg/config"
	"go.uber.org/zap"
)

// batchLimit caps how many messages one collect callback hands to Fluent Bit.
const batchLimit = 100

// Global plugin state
var (
	logger            *zap.SugaredLogger
	consumer          *amqp.Consumer
	cancelConsumer    context.CancelFunc
	decodeCloudEvents bool
)

func initLogger() *zap.SugaredLogger {
	zapLogger, err := zap.NewProduction()
	if err != nil {
		return zap.NewNop().Sugar()
	}
	return zapLogger.Sugar()
}

//export FLBPluginRegister
func FLBPluginRegister(def unsafe.Pointer) int {
	logger = initLogger()
	return input.FLBPluginRegisterWithOptions(def,
		input.WithName("amqp"),
		input.WithDescription("Consume records from an AMQP queue (plain JSON or CloudEvents)"),
	)
}

//export FLBPluginInit
func FLBPluginInit(plugin unsafe.Pointer) int {
	cfg := config.DefaultConfig()
	if err := cfg.LoadFromLookup(func(key string) string {
		return input.FLBPluginConfigKey(plugin, key)
	}); err != nil {
		logger.Errorf("Failed to load configuration: %v", err)
		return input.FLB_ERROR
	}

	c, err := amqp.NewConsumer(cfg)
	if err != nil {
		logger.Errorf("Failed to initialize AMQP consumer: %v", err)
		return input.FLB_ERROR
	}
	c.SetLogger(logger)

	ctx, cancel := context.WithCancel(context.Background())
	c.Start(ctx)

	consumer = c
	cancelConsumer = cancel
	decodeCloudEvents = cfg.CloudEventsDecode

	logger.Infow("AMQP input plugin initialized",
		"queue", cfg.Queue,
		"exchange", cfg.Exchange,
		"routing_key", cfg.RoutingKey,
		"prefetch_count", cfg.PrefetchCount,
		"cloudevents_decode", cfg.CloudEventsDecode)
	return input.FLB_OK
}

// FLBPluginInputCallback is called by Fluent Bit on each collection interval. It
// returns a msgpack stream of [timestamp, record] events for the buffered messages.
//
//export FLBPluginInputCallback
func FLBPluginInputCallback(data *unsafe.Pointer, size *C.size_t) int {
	*data = nil
	*size = 0
	if consumer == nil {
		return input.FLB_OK
	}

	batch := consumer.Next(batchLimit)
	if len(batch) == 0 {
		return input.FLB_OK
	}

	enc := input.NewEncoder()
	var payload []byte
	accepted := make([]amqpbroker.Delivery, 0, len(batch))
	for _, d := range batch {
		ts := d.Timestamp
		if ts.IsZero() {
			ts = time.Now()
		}
		record := amqp.RecordFromBody(d.Body, decodeCloudEvents)
		packed, err := enc.Encode([]interface{}{&input.FLBTime{Time: ts}, record})
		if err != nil {
			logger.Errorw("Failed to encode message; rejecting without requeue", "error", err)
			if rejectErr := d.Reject(false); rejectErr != nil {
				logger.Errorw("Failed to reject message", "error", rejectErr)
			}
			continue
		}
		payload = append(payload, packed...)
		accepted = append(accepted, d)
	}

	if len(payload) > 0 {
		*data = C.CBytes(payload)
		*size = C.size_t(len(payload))
	}

	// The batch is acknowledged once it has been handed to Fluent Bit. A crash between
	// here and the engine accepting it can lose the batch.
	if err := amqp.AckAll(accepted); err != nil {
		logger.Errorw("Failed to acknowledge messages", "error", err)
	}
	return input.FLB_OK
}

//export FLBPluginExit
func FLBPluginExit() int {
	if cancelConsumer != nil {
		cancelConsumer()
	}
	return input.FLB_OK
}

func main() {
}
