package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	olog "go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/log/global"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
)

var (
	resource          *sdkresource.Resource
	initResourcesOnce sync.Once

	cancelContextWith context.CancelCauseFunc

	logger = global.GetLoggerProvider().Logger("")
)

func main() {
	if err := run(); err != nil {
		fmt.Printf("Failed to run log generator: %v\n", err)
		os.Exit(1)
	}
}

func run() (err error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)

	shutdown, err := setupOTelSDK(ctx)
	if err != nil {
		return fmt.Errorf("failed to setup OpenTelemetry SDK: %v", err)
	}

	defer func() {
		if err := shutdown(ctx); err != nil {
			fmt.Printf("failed to shutdown OpenTelemetry SDK: %v", err)
		}
	}()

	done := make(chan struct{})
	go func() {
		fmt.Println("Starting log emission...")

		logCount := 10
		for i := 1; i <= logCount; i++ {
			func() {
				rec := olog.Record{}
				rec.SetSeverity(olog.SeverityInfo)
				rec.SetBody(olog.StringValue("test"))
				rec.AddAttributes(olog.KeyValueFromAttribute(attribute.String("log-count", strconv.Itoa(i))))
				logger.Emit(context.Background(), rec)
				time.Sleep(10 * time.Millisecond)
			}()
		}
		close(done)
	}()

	select {
	case <-stop:
		log.Println("Interrupt signal received. Shutting down...")
	case <-done:
		log.Println("Completed log emission. Shutting down...")
	}

	return nil
}

func setupOTelSDK(ctx context.Context) (shutdown func(context.Context) error, err error) {
	var shutdownFuncs []func(context.Context) error

	shutdown = func(ctx context.Context) error {
		var err error
		for _, fn := range shutdownFuncs {
			err = errors.Join(err, fn(ctx))
		}
		shutdownFuncs = nil
		return err
	}

	handleErr := func(inErr error) {
		err = errors.Join(inErr, shutdown(ctx))
	}

	loggerProvider, err := newLoggerProvider(ctx)
	if err != nil {
		handleErr(err)
		return
	}
	shutdownFuncs = append(shutdownFuncs, loggerProvider.Shutdown)
	global.SetLoggerProvider(loggerProvider)

	return shutdown, err
}

func newLoggerProvider(ctx context.Context) (*sdklog.LoggerProvider, error) {
	grpcExporter, err := otlploggrpc.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create gRPC exporter: %w", err)
	}

	loggerProvider := sdklog.NewLoggerProvider(
		sdklog.WithProcessor(sdklog.NewSimpleProcessor(grpcExporter)),
	)

	return loggerProvider, nil
}
