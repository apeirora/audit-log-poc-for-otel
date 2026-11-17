package main

import (
	"log"
	"net/http"
	"os"

	"audit-loss/internal/healthcheck"
	"audit-loss/internal/spammer"
)

func main() {
	healthcheck.MaybeExit()
	log.SetPrefix("spammer ")
	addr := env("LISTEN", ":8080")
	srv := &spammer.Server{OTLPURL: env("OTLP_URL", "http://127.0.0.1:4318/v1/logs")}
	log.Printf("listening %s otlp %s", addr, srv.OTLPURL)
	log.Fatal(http.ListenAndServe(addr, srv.Handler()))
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
