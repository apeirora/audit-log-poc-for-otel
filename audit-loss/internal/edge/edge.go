package edge

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	"audit-loss/internal/integrity"
	"audit-loss/internal/otlpjson"
)

func SignHandler(key []byte, upstream string, client *http.Client) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 8<<20))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		doc, err := otlpjson.Parse(body)
		if err != nil {
			http.Error(w, "malformed log", http.StatusBadRequest)
			return
		}
		otlpjson.Walk(doc, func(rec map[string]any) {
			sig := integrity.Sign(key,
				otlpjson.GetAttr(rec, "demo.run_id"),
				otlpjson.GetAttr(rec, "demo.sequence"),
				otlpjson.GetAttr(rec, "demo.emit_time_unix_nano"),
				otlpjson.Body(rec),
			)
			if otlpjson.GetAttr(rec, "demo.inject") == "bad_signature" && len(sig) > 0 {
				sig = flipHex(sig)
			}
			otlpjson.SetAttr(rec, "audit.integrity.value", sig)
		})
		raw, err := otlpjson.Marshal(doc)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		forward(w, client, upstream, r.URL.RequestURI(), raw)
	})
}

func VerifyHandler(key []byte, upstream, sinkURL string, client *http.Client) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 8<<20))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		doc, err := otlpjson.Parse(body)
		if err != nil {
			http.Error(w, "malformed log", http.StatusBadRequest)
			return
		}
		kept := otlpjson.Filter(doc, func(rec map[string]any) bool {
			if missingRequired(rec) {
				report(client, sinkURL, otlpjson.GetAttr(rec, "demo.run_id"), otlpjson.GetAttr(rec, "demo.sequence"), "missing_attributes")
				return false
			}
			ok := integrity.Verify(key,
				otlpjson.GetAttr(rec, "demo.run_id"),
				otlpjson.GetAttr(rec, "demo.sequence"),
				otlpjson.GetAttr(rec, "demo.emit_time_unix_nano"),
				otlpjson.Body(rec),
				otlpjson.GetAttr(rec, "audit.integrity.value"),
			)
			if !ok {
				report(client, sinkURL, otlpjson.GetAttr(rec, "demo.run_id"), otlpjson.GetAttr(rec, "demo.sequence"), "bad_signature")
			}
			return ok
		})
		if kept == 0 {
			w.WriteHeader(http.StatusOK)
			return
		}
		raw, err := otlpjson.Marshal(doc)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		forward(w, client, upstream, r.URL.RequestURI(), raw)
	})
}

func flipHex(sig string) string {
	b := []byte(sig)
	if b[len(b)-1] == '0' {
		b[len(b)-1] = '1'
	} else {
		b[len(b)-1] = '0'
	}
	return string(b)
}

func forward(w http.ResponseWriter, client *http.Client, upstream, path string, body []byte) {
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(upstream, "/")+path, bytes.NewReader(body))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func missingRequired(rec map[string]any) bool {
	for _, key := range []string{"demo.run_id", "demo.sequence", "demo.emit_time_unix_nano", "demo.scenario"} {
		if otlpjson.GetAttr(rec, key) == "" {
			return true
		}
	}
	return false
}

func report(client *http.Client, sinkURL, runID, sequence, reason string) {
	if sinkURL == "" || runID == "" {
		return
	}
	seq, _ := strconv.Atoi(sequence)
	raw, _ := json.Marshal(map[string]any{"run_id": runID, "sequence": seq, "reason": reason})
	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(sinkURL, "/")+"/rejected", bytes.NewReader(raw))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("report rejection: %v", err)
		return
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
}
