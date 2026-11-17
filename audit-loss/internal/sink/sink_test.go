package sink

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"audit-loss/internal/otlpjson"
)

func TestIngestDuplicateAndStats(t *testing.T) {
	dir := t.TempDir()
	start := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	finish := start.Add(time.Second)
	spammer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"run_id":      "run1",
			"sent":        2,
			"rejected":    0,
			"started_at":  start.Format(time.RFC3339Nano),
			"finished_at": finish.Format(time.RFC3339Nano),
		})
	}))
	defer spammer.Close()

	store := New(dir, spammer.URL, time.Second)
	now := finish.Add(2 * time.Second)
	store.now = func() time.Time { return now }

	body := mustLogs(t, "run1", "1", "main")
	post(t, store, "/v1/logs", body)
	post(t, store, "/v1/logs", body)
	now = now.Add(2 * time.Second)

	rec := httptest.NewRequest(http.MethodGet, "/stats?run_id=run1", nil)
	rr := httptest.NewRecorder()
	store.Handler().ServeHTTP(rr, rec)
	var stats Stats
	if err := json.Unmarshal(rr.Body.Bytes(), &stats); err != nil {
		t.Fatal(err)
	}
	if stats.Received != 1 || stats.Duplicates != 1 || stats.ByCollector["main"] != 1 {
		t.Fatalf("stats %+v", stats)
	}
	if stats.Status != "drained" {
		t.Fatalf("status %s", stats.Status)
	}
	if len(stats.Missing) != 1 || stats.Missing[0] != 2 {
		t.Fatalf("missing %+v", stats.Missing)
	}
	if _, err := os.Stat(filepath.Join(dir, "run1", "summary.json")); err != nil {
		t.Fatal(err)
	}
}

func TestRunningDoesNotCallGapsLoss(t *testing.T) {
	dir := t.TempDir()
	store := New(dir, "", time.Second)
	store.now = time.Now
	post(t, store, "/v1/logs", mustLogs(t, "run2", "1", "secondary"))
	req := httptest.NewRequest(http.MethodGet, "/stats?run_id=run2", nil)
	rr := httptest.NewRecorder()
	store.Handler().ServeHTTP(rr, req)
	var stats Stats
	if err := json.Unmarshal(rr.Body.Bytes(), &stats); err != nil {
		t.Fatal(err)
	}
	if stats.Status != "running" || len(stats.Missing) != 0 {
		t.Fatalf("stats %+v", stats)
	}
}

func TestPercentile(t *testing.T) {
	got := latencyStats([]float64{1, 2, 3, 4, 10})
	if got.Min != 1 || got.Max != 10 || got.P50 != 3 || got.P95 != 10 || got.Avg != 4 {
		t.Fatalf("%+v", got)
	}
}

func post(t *testing.T, store *Store, path string, body []byte) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	rr := httptest.NewRecorder()
	store.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("%s %d %s", path, rr.Code, rr.Body.String())
	}
}

func mustLogs(t *testing.T, runID, seq, collector string) []byte {
	t.Helper()
	raw, err := otlpjson.Build([]otlpjson.Record{{
		Sequence: seq,
		Scenario: "happy",
		RunID:    runID,
		Emit:     "100",
		Body:     "audit log",
		Attrs:    map[string]string{"demo.collector": collector},
	}})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
