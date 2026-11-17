package main

import (
	"log"
	"net/http"
	"os"

	"audit-loss/internal/healthcheck"
	"audit-loss/internal/toxic"
)

func main() {
	healthcheck.MaybeExit()
	log.SetPrefix("toxic ")
	srv := toxic.New(map[string]string{
		"to-main":      env("UPSTREAM_TO_MAIN", "http://127.0.0.1:4318"),
		"to-secondary": env("UPSTREAM_TO_SECONDARY", "http://127.0.0.1:4319"),
		"to-sink":      env("UPSTREAM_TO_SINK", "http://127.0.0.1:8080"),
	})
	go serve(env("LINK_TO_MAIN", ":4417"), srv.Proxy("to-main"))
	go serve(env("LINK_TO_SECONDARY", ":4418"), srv.Proxy("to-secondary"))
	go serve(env("LINK_TO_SINK", ":4419"), srv.Proxy("to-sink"))
	addr := env("LISTEN", ":8090")
	log.Printf("control %s", addr)
	log.Fatal(http.ListenAndServe(addr, srv.Control()))
}

func serve(addr string, h http.Handler) {
	log.Printf("proxy %s", addr)
	log.Fatal(http.ListenAndServe(addr, h))
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
