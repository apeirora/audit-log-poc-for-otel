package main

import (
	"context"
	"io"
	"log"
	"net"
	"net/http"
	"os"

	collectorlogsv1 "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/encoding/gzip"
	"google.golang.org/protobuf/proto"
)

type logServer struct {
	collectorlogsv1.LogsServiceServer
	file *os.File
}

func (s *logServer) Export(ctx context.Context, req *collectorlogsv1.ExportLogsServiceRequest) (*collectorlogsv1.ExportLogsServiceResponse, error) {
	for _, rl := range req.ResourceLogs {
		for _, sl := range rl.ScopeLogs {
			for _, l := range sl.LogRecords {
				_, err := s.file.WriteString(l.String() + "\n")
				if err != nil {
					return &collectorlogsv1.ExportLogsServiceResponse{}, err
				}
			}
		}
	}
	return &collectorlogsv1.ExportLogsServiceResponse{}, nil
}

func main() {
	// Register gzip decompressor
	_ = gzip.Name // Ensures gzip is registered

	lis, _ := net.Listen("tcp", ":5317")
	f, _ := os.Create("received-logs.txt")
	s := grpc.NewServer()
	collectorlogsv1.RegisterLogsServiceServer(s, &logServer{file: f})

	go func() {
		http.Handle("/v1/logs", &logServer{file: f})
		log.Println("HTTP listening on :5318")
		log.Fatal(http.ListenAndServe(":5318", nil))
	}()

	log.Println("gRPC listening on :5317")
	err := s.Serve(lis)
	panic(err)
}

func (s *logServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// log all incoming requests
	log.Printf("Received HTTP request: %s %s", r.Method, r.URL.Path)
	// log all headers
	for name, values := range r.Header {
		for _, value := range values {
			log.Printf("%s: %s", name, value)
		}
	}

	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			panic(err)
		}
	}(r.Body)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}

	var req collectorlogsv1.ExportLogsServiceRequest
	if err := proto.Unmarshal(body, &req); err != nil {
		http.Error(w, "Failed to parse protobuf", http.StatusBadRequest)
		return
	}
	// Schreibe Logs wie im gRPC-Handler
	for _, rl := range req.ResourceLogs {
		for _, sl := range rl.ScopeLogs {
			for _, l := range sl.LogRecords {
				_, err := s.file.WriteString(l.String() + "\n") // FIXME: format logs better, so that we can check easily if they are the same as they have been created in the clients
				if err != nil {
					http.Error(w, "Failed to write to file", http.StatusInternalServerError)
				}
			}
		}
	}
	// Leere Antwort im Protobuf-Format
	resp := &collectorlogsv1.ExportLogsServiceResponse{}
	out, _ := proto.Marshal(resp)
	w.Header().Set("Content-Type", "application/x-protobuf")
	w.WriteHeader(http.StatusOK)
	_, err = w.Write(out)
	if err != nil {
		http.Error(w, "Failed to write response", http.StatusInternalServerError)
	}
}
