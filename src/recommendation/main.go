package main

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"os"
	"sync"

	pb "github.com/open-telemetry/opentelemetry-demo/src/checkout/genproto/oteldemo"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutlog"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
)

var (
	resource          *sdkresource.Resource
	initResourcesOnce sync.Once

	tracer = otel.Tracer("")
	meter  = otel.Meter("")
	logger = global.GetLoggerProvider().Logger("")
)

type recommendationService struct {
	pb.UnimplementedRecommendationServiceServer

	catalogClient pb.ProductCatalogServiceClient
}

func main() {
	if err := run(); err != nil {
		fmt.Printf("Failed to run recommendation service: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	shutdown, err := setupOTelSDK(context.Background())
	if err != nil {
		return fmt.Errorf("failed to setup OpenTelemetry SDK: %v", err)
	}
	defer func() {
		if err := shutdown(context.Background()); err != nil {
			fmt.Printf("failed to shutdown OpenTelemetry SDK: %v", err)
		}
	}()

	catalogClientAddr, err := getEnvVar("PRODUCT_CATALOG_ADDR")
	if err != nil {
		return err
	}

	c, err := createClient(catalogClientAddr)
	if err != nil {
		return err
	}

	defer c.Close()

	svc := &recommendationService{
		catalogClient: pb.NewProductCatalogServiceClient(c),
	}

	port, err := getEnvVar("RECOMMENDATION_PORT")
	if err != nil {
		return err
	}

	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", port))
	if err != nil {
		return fmt.Errorf("failed to listen: %v", err)
	}

	var srv = grpc.NewServer()

	pb.RegisterRecommendationServiceServer(srv, svc)
	healthpb.RegisterHealthServer(srv, svc)

	return srv.Serve(lis)
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

	// Set up propagator.
	prop := newPropagator()
	otel.SetTextMapPropagator(prop)

	// Set up trace provider.
	tracerProvider, err := newTracerProvider()
	if err != nil {
		handleErr(err)
		return
	}
	shutdownFuncs = append(shutdownFuncs, tracerProvider.Shutdown)
	otel.SetTracerProvider(tracerProvider)

	// Set up meter provider.
	meterProvider, err := newMeterProvider()
	if err != nil {
		handleErr(err)
		return
	}
	shutdownFuncs = append(shutdownFuncs, meterProvider.Shutdown)
	otel.SetMeterProvider(meterProvider)

	// Set up logger provider.
	loggerProvider, err := newLoggerProvider()
	if err != nil {
		handleErr(err)
		return
	}
	shutdownFuncs = append(shutdownFuncs, loggerProvider.Shutdown)
	global.SetLoggerProvider(loggerProvider)

	return
}

func newPropagator() propagation.TextMapPropagator {
	return propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)
}

func newTracerProvider() (*sdktrace.TracerProvider, error) {
	traceExporter, err := stdouttrace.New(
		stdouttrace.WithPrettyPrint())
	if err != nil {
		return nil, err
	}

	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter),
		sdktrace.WithResource(initResource()),
	)
	return tracerProvider, nil
}

func newMeterProvider() (*sdkmetric.MeterProvider, error) {
	metricExporter, err := stdoutmetric.New()
	if err != nil {
		return nil, err
	}

	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter)),
		sdkmetric.WithResource(initResource()),
	)
	return meterProvider, nil
}

func newLoggerProvider() (*sdklog.LoggerProvider, error) {
	logExporter, err := stdoutlog.New()
	if err != nil {
		return nil, err
	}

	loggerProvider := sdklog.NewLoggerProvider(
		sdklog.WithProcessor(sdklog.NewBatchProcessor(logExporter)),
		sdklog.WithResource(initResource()),
	)
	return loggerProvider, nil
}

func initResource() *sdkresource.Resource {
	initResourcesOnce.Do(func() {
		extraResources, _ := sdkresource.New(
			context.Background(),
			sdkresource.WithOS(),
			sdkresource.WithProcess(),
			sdkresource.WithContainer(),
			sdkresource.WithHost(),
		)
		resource, _ = sdkresource.Merge(
			sdkresource.Default(),
			extraResources,
		)
	})
	return resource
}

func getEnvVar(envKey string) (string, error) {
	v := os.Getenv(envKey)
	if v == "" {
		return "", fmt.Errorf("environment variable %s not set", envKey)
	}

	return v, nil
}

func createClient(svcAddr string) (*grpc.ClientConn, error) {
	c, err := grpc.NewClient(svcAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("could not connect to %s service, err: %+v", svcAddr, err)
	}

	return c, err
}

func (s *recommendationService) ListRecommendations(ctx context.Context, req *pb.ListRecommendationsRequest) (*pb.ListRecommendationsResponse, error) {
	rec := log.Record{}
	rec.SetBody(log.StringValue("ListRecommendations request"))
	rec.AddAttributes(
		log.KeyValueFromAttribute(attribute.StringSlice("product_ids", req.GetProductIds())),
	)
	logger.Emit(ctx, log.Record(rec))

	resp, err := s.catalogClient.ListProducts(context.Background(), &pb.Empty{})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list products: %v", err)
	}

	var allProductIDs []string
	for _, product := range resp.GetProducts() {
		allProductIDs = append(allProductIDs, product.GetId())
	}

	rec = log.Record{}
	rec.SetBody(log.StringValue("ListRecommendations product catalog query"))
	rec.AddAttributes(
		log.KeyValueFromAttribute(attribute.StringSlice("product_ids", allProductIDs)),
	)
	logger.Emit(ctx, log.Record(rec))

	inputSet := make(map[string]struct{})
	for _, id := range req.GetProductIds() {
		inputSet[id] = struct{}{}
	}

	// Create a filtered list of products excluding the products received as input
	var filtered []string
	for _, id := range allProductIDs {
		if _, found := inputSet[id]; !found {
			filtered = append(filtered, id)
		}
	}

	// Shuffle and return up to maxResponses
	const maxResponses = 5
	rand.Shuffle(len(filtered), func(i, j int) { filtered[i], filtered[j] = filtered[j], filtered[i] })
	if len(filtered) > maxResponses {
		filtered = filtered[:maxResponses]
	}

	rec = log.Record{}
	rec.SetBody(log.StringValue("ListRecommendations response"))
	rec.AddAttributes(
		log.KeyValueFromAttribute(attribute.StringSlice("product_ids", filtered)),
	)
	logger.Emit(ctx, log.Record(rec))

	return &pb.ListRecommendationsResponse{
		ProductIds: filtered,
	}, nil
}

func (s *recommendationService) Check(ctx context.Context, _ *healthpb.HealthCheckRequest) (*healthpb.HealthCheckResponse, error) {
	return &healthpb.HealthCheckResponse{Status: healthpb.HealthCheckResponse_SERVING}, nil
}

func (s *recommendationService) List(context.Context, *healthpb.HealthListRequest) (*healthpb.HealthListResponse, error) {
	return &healthpb.HealthListResponse{}, nil
}

func (s *recommendationService) Watch(_ *healthpb.HealthCheckRequest, _ healthpb.Health_WatchServer) error {
	return nil // Not implemented
}
