package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"audit-loss/internal/edge"
	"audit-loss/internal/healthcheck"
)

func main() {
	healthcheck.MaybeExit()
	modeFlag := flag.String("mode", os.Getenv("MODE"), "sign or verify")
	flag.Parse()
	mode := *modeFlag
	log.SetPrefix("edge ")
	key, err := os.ReadFile(env("KEY_FILE", "keys/dev_hmac_key.txt"))
	if err != nil {
		log.Fatal(err)
	}
	client := &http.Client{Timeout: 10 * time.Second}
	listen := env("LISTEN", ":4318")
	upstream := env("UPSTREAM", "http://127.0.0.1:4319")
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	switch mode {
	case "sign":
		mux.Handle("POST /v1/logs", edge.SignHandler(bytesTrim(key), upstream, client))
	case "verify":
		mux.Handle("POST /v1/logs", edge.VerifyHandler(bytesTrim(key), upstream, env("SINK_URL", ""), client))
	default:
		log.Fatal("MODE must be sign or verify")
	}
	log.Printf("%s listening %s upstream %s", mode, listen, upstream)
	log.Fatal(http.ListenAndServe(listen, mux))
}

func bytesTrim(b []byte) []byte {
	return []byte(strings.TrimSpace(string(b)))
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
