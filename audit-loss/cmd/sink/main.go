package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"audit-loss/internal/healthcheck"
	"audit-loss/internal/sink"
)

func main() {
	healthcheck.MaybeExit()
	log.SetPrefix("sink ")
	addr := env("LISTEN", ":8080")
	drain, err := time.ParseDuration(env("DRAIN", "2s"))
	if err != nil {
		log.Fatal(err)
	}
	dir := env("SINK_DIR", "data")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Fatal(err)
	}
	store := sink.New(dir, os.Getenv("SPAMMER_URL"), drain)
	log.Printf("listening %s dir %s", addr, dir)
	log.Fatal(http.ListenAndServe(addr, store.Handler()))
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
