package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"time"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutlog"
	olog "go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/log/global"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
)

var (
	resource          *sdkresource.Resource
	initResourcesOnce sync.Once

	cancelContextWith context.CancelCauseFunc

	logger  = global.GetLoggerProvider().Logger("unused")
	slogger = otelslog.NewLogger("AUDIT-otelslog")
)

func main() {
	if err := run(); err != nil {
		fmt.Printf("Failed to run recommendation service: %v\n", err)
		log.Fatalln(err)
		os.Exit(1)
	}
}

func run() (err error) {
	// Handle SIGINT (CTRL+C) gracefully.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	shutdown, err := setupOTelSDK(ctx)
	if err != nil {
		defer cancelContextWith(err)
		return fmt.Errorf("failed to setup OpenTelemetry SDK: %v", err)
	}
	defer func() {
		if err := shutdown(context.Background()); err != nil {
			defer cancelContextWith(err)
			fmt.Printf("failed to shutdown OpenTelemetry SDK: %v", err)
		}
	}()

	// Start HTTP server.
	srv := &http.Server{
		Addr:         ":8081",
		BaseContext:  func(_ net.Listener) context.Context { return ctx },
		ReadTimeout:  time.Second,
		WriteTimeout: 10 * time.Second,
		Handler:      newHTTPHandler(),
	}
	srvErr := make(chan error, 1)
	go func() {
		srvErr <- srv.ListenAndServe()
	}()

	// Wait for interruption.
	select {
	case err = <-srvErr:
		// Error when starting HTTP server.
		return
	case <-ctx.Done():
		// Wait for first CTRL+C.
		// Stop receiving signal notifications as soon as possible.
		stop()
	}

	// When Shutdown is called, ListenAndServe immediately returns ErrServerClosed.
	err = srv.Shutdown(context.Background())

	return
}

func rolldice(w http.ResponseWriter, r *http.Request) {
	roll := 1 + rand.Intn(6)

	var msg string
	if player := r.PathValue("player"); player != "" {
		msg = fmt.Sprintf("%s is rolling the dice", player)
	} else {
		msg = "Anonymous player is rolling the dice"
	}
	slogger.InfoContext(r.Context(), msg, "result", roll, "AUDIT-USER", r.PathValue("player"))

	resp := strconv.Itoa(roll) + "\n"
	if _, err := io.WriteString(w, resp); err != nil {
		log.Printf("Write failed: %v\n", err)
	}
}

func newHTTPHandler() http.Handler {
	mux := http.NewServeMux()

	// handleFunc is a replacement for mux.HandleFunc
	// which enriches the handler's HTTP instrumentation with the pattern as the http.route.
	handleFunc := func(pattern string, handlerFunc func(http.ResponseWriter, *http.Request)) {
		// Configure the "http.route" for the HTTP instrumentation.
		handler := otelhttp.WithRouteTag(pattern, http.HandlerFunc(handlerFunc))
		mux.Handle(pattern, handler)
	}

	// Register handlers.
	handleFunc("/rolldice/", rolldice)
	handleFunc("/rolldice/{player}", rolldice)

	// Add HTTP instrumentation for the whole server.
	handler := otelhttp.NewHandler(mux, "/")
	return handler
}

// setupOTelSDK bootstraps the OpenTelemetry pipeline.
// If it does not return an error, make sure to call shutdown for proper cleanup.
func setupOTelSDK(ctx context.Context) (shutdown func(context.Context) error, err error) {
	var shutdownFuncs []func(context.Context) error

	// shutdown calls cleanup functions registered via shutdownFuncs.
	// The errors from the calls are joined.
	// Each registered cleanup will be invoked once.
	shutdown = func(ctx context.Context) error {
		var err error
		for _, fn := range shutdownFuncs {
			err = errors.Join(err, fn(ctx))
		}
		shutdownFuncs = nil
		return err
	}

	// handleErr calls shutdown for cleanup and makes sure that all errors are returned.
	handleErr := func(inErr error) {
		err = errors.Join(inErr, shutdown(ctx))
	}

	// Set up logger provider.
	loggerProvider, err := newLoggerProvider(ctx)
	if err != nil {
		handleErr(err)
		return
	}
	shutdownFuncs = append(shutdownFuncs, loggerProvider.Shutdown)
	global.SetLoggerProvider(loggerProvider)

	otel.GetErrorHandler().Handle(errors.New("handle test error 1"))
	// Set up the OpenTelemetry error handler.

	otel.SetErrorHandler(&customErrorHandler{})

	otel.GetErrorHandler().Handle(errors.New("handle test error 2"))

	// otel-collector        ClusterIP   10.43.238.160   <none>        6831/UDP,14250/TCP,14268/TCP,8888/TCP,4317/TCP,4318/TCP,9411/TCP   25h
	// kubectl get services --namespace otel-demo
	// kubectl describe service otel-collector --namespace otel-demo
	/*
		Port:                     otlp  4317/TCP
		TargetPort:               4317/TCP
		Endpoints:                10.42.0.81:4317

		Port:                     otlp-http  4318/TCP
		TargetPort:               4318/TCP
		Endpoints:                10.42.0.81:4318
	*/

	logger = loggerProvider.Logger("AUDIT_RECOMMENDATION_SERVICE", // We can set a custom logger name for AUDIT purposes
		olog.WithInstrumentationAttributes(attribute.String("AUDIT", "RECOMMENDATION_SERVICE"))) // We can set a custom attributes for AUDIT purposes

	rec := olog.Record{}
	rec.SetSeverity(olog.SeverityInfo)
	rec.SetBody(olog.StringValue("test log message: Recommendation service started"))
	rec.AddAttributes(olog.KeyValueFromAttribute(attribute.String("AUDIT-key-2", "KeyValueFromAttribute")))
	logger.Emit(ctx, rec)

	return shutdown, err
}

// newLoggerProvider creates a new logger provider with OTLP exporters and stdout exporter.
// It requires the OTEL_EXPORTER_OTLP_ENDPOINT_GRPC and OTEL_EXPORTER_OTLP_ENDPOINT_HTTP environment variables to be set.
func newLoggerProvider(ctx context.Context) (*sdklog.LoggerProvider, error) {
	grpcEndpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT_GRPC")
	if grpcEndpoint == "" {
		grpcEndpoint = "localhost:4317" // Default gRPC endpoint
	}
	grpcExporter, err := otlploggrpc.New(ctx, otlploggrpc.WithEndpoint(grpcEndpoint),
		otlploggrpc.WithInsecure( /* fixes "transport: authentication handshake failed: tls: first record does not look like a TLS handshake" */ ))
	if err != nil {
		return nil, err
	}

	httpEndpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT_HTTP")
	if httpEndpoint == "" {
		httpEndpoint = "localhost:4318" // Default HTTP endpoint
	}
	httpExporter, err := otlploghttp.New(ctx, otlploghttp.WithEndpoint(httpEndpoint), otlploghttp.WithInsecure( /* don't know how to setup https */ ))
	if err != nil {
		return nil, err
	}

	stdoutExporter, err := stdoutlog.New(stdoutlog.WithPrettyPrint(), stdoutlog.WithoutTimestamps())
	if err != nil {
		return nil, err
	}

	loggerProvider := sdklog.NewLoggerProvider(
		sdklog.WithProcessor(sdklog.NewSimpleProcessor(stdoutExporter)),
		sdklog.WithProcessor(sdklog.NewSimpleProcessor(httpExporter)),
		sdklog.WithProcessor(sdklog.NewSimpleProcessor(grpcExporter)),
		sdklog.WithResource(initResource()),
	)
	return loggerProvider, nil
}

func initResource() *sdkresource.Resource {
	var ctx context.Context
	ctx, cancelContextWith = context.WithCancelCause(context.Background())

	// defer cancelContextWith(errors.New("initializing resource"))

	initResourcesOnce.Do(func() {
		err := context.Cause(ctx)
		errTxt := ""
		if err != nil {
			errTxt = err.Error()
		}
		extraResources, _ := sdkresource.New(
			ctx, sdkresource.WithAttributes(attribute.String("AUDIT-contextCause", errTxt)),
		)
		resource, _ = sdkresource.Merge(
			sdkresource.Empty(),
			extraResources,
		)
	})
	return resource
}

// customErrorHandler implements otel.ErrorHandler.
type customErrorHandler struct{}

func (h *customErrorHandler) Handle(err error) {
	// Custom error handling doesn't really help to guarantee any log delivery, because we don't know what happened to the log record.
	fmt.Printf("My-OpenTelemetry error: %v\n", err)
}
