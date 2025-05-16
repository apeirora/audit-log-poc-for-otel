package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net"
	"os"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"

	pb "github.com/apeirora/audit-log-poc-for-otel/recommendation/genproto/oteldemo"
)

type recommendation struct {
	pb.UnimplementedRecommendationServiceServer

	catalogClient pb.ProductCatalogServiceClient
	mu            sync.Mutex
	cachedIDs     []string
	firstRun      bool
}

func main() {
	if err := run(); err != nil {
		log.Fatalf("Failed to run recommendation service: %v", err)
	}
}

func run() error {
	catalogClientAddr, err := getEnvVar("PRODUCT_CATALOG_ADDR")
	if err != nil {
		return err
	}

	c, err := createClient(catalogClientAddr)
	if err != nil {
		return err
	}

	defer c.Close()

	svc := &recommendation{
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

func (s *recommendation) ListRecommendations(ctx context.Context, req *pb.ListRecommendationsRequest) (*pb.ListRecommendationsResponse, error) {
	log.Printf("ListRecommendations: received request with %+v", req.GetProductIds())

	resp, err := s.catalogClient.ListProducts(context.Background(), &pb.Empty{})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list products: %v", err)
	}

	var allProductIDs []string
	for _, product := range resp.GetProducts() {
		allProductIDs = append(allProductIDs, product.GetId())
	}

	log.Printf("ListRecommendations: all product IDs from product catalog: %+v", allProductIDs)

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

	log.Printf("ListRecommendations: responding with %+v", filtered)

	return &pb.ListRecommendationsResponse{
		ProductIds: filtered,
	}, nil
}

func (s *recommendation) Check(ctx context.Context, _ *healthpb.HealthCheckRequest) (*healthpb.HealthCheckResponse, error) {
	return &healthpb.HealthCheckResponse{Status: healthpb.HealthCheckResponse_SERVING}, nil
}

func (s *recommendation) List(context.Context, *healthpb.HealthListRequest) (*healthpb.HealthListResponse, error) {
	return &healthpb.HealthListResponse{}, nil
}

func (s *recommendation) Watch(_ *healthpb.HealthCheckRequest, _ healthpb.Health_WatchServer) error {
	return nil // Not implemented
}
