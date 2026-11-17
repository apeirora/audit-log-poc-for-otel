package main

import (
	"log"
	"net/http"
	"os"

	"audit-loss/internal/healthcheck"
	"audit-loss/internal/ui"
)

func main() {
	healthcheck.MaybeExit()
	log.SetPrefix("ui ")
	srv := &ui.Server{
		SpammerURL:      env("SPAMMER_URL", "http://log-spammer:8080"),
		ToxicURL:        env("TOXIC_URL", "http://toxic:8090"),
		SinkURL:         env("SINK_URL", "http://audit-sink:8080"),
		Main:            env("MAIN_CONTAINER", "audit-main"),
		Secondary:       env("SECONDARY_CONTAINER", "audit-secondary"),
		Spammer:         env("SPAMMER_CONTAINER", "audit-spammer"),
		Sidecar:         env("SIDECAR_CONTAINER", "audit-loss-otel-collector-audit-sidecar-1"),
		MainHealth:      env("MAIN_HEALTH", "http://otel-collector-audit-main:13133/"),
		SecondaryHealth: env("SECONDARY_HEALTH", "http://otel-collector-audit-secondary:13133/"),
		SidecarHealth:   env("SIDECAR_HEALTH", "http://log-spammer:13133/"),
		Docker:          ui.NewDocker(env("DOCKER_SOCKET", "/var/run/docker.sock")),
	}
	addr := env("LISTEN", ":8080")
	log.Printf("listening %s", addr)
	log.Fatal(http.ListenAndServe(addr, srv.Handler()))
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
