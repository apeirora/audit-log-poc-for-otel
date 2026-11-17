package ui

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"audit-loss/internal/sink"
)

func TestLabelsNameThePlace(t *testing.T) {
	rr := httptest.NewRecorder()
	(&Server{}).Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	body := rr.Body.String()
	faults := []string{"malformed_json", "missing_attrs", "bad_timestamp", "oversize", "duplicate_id", "connection_reset", "bad_signature", "bad_in_batch", "stall", "slow"}
	for _, fault := range faults {
		if !bytes.Contains([]byte(body), []byte(faultLabel(fault))) {
			t.Fatalf("page missing fault label %s", faultLabel(fault))
		}
	}
	actions := []string{"kill-main", "stop-main", "start-main", "kill-secondary", "stop-secondary", "start-secondary", "blackhole-main", "blackhole-secondary", "clear-faults", "low-bandwidth-main", "low-bandwidth-secondary", "low-bandwidth-sink", "restart-sidecar"}
	for _, action := range actions {
		if !bytes.Contains([]byte(body), []byte(actionLabel(action))) {
			t.Fatalf("page missing action label %s", actionLabel(action))
		}
	}
	got := describe(
		[]injection{{Every: 2, Fault: "bad_signature"}},
		[]runStep{{After: 5, Action: "blackhole-main"}},
	)
	want := "Every 2 logs: bad signature, rejected at verify. After 5 logs: blackhole on path to main. "
	if got != want {
		t.Fatalf("describe %q", got)
	}
}

func TestPageListsScenarioButtons(t *testing.T) {
	srv := &Server{}
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	body := rr.Body.String()
	for _, label := range []string{"Audit log demo", "Delivery", "Failover", "Loss", "Missing sequences", "Via main", "Every Nth problem", "After N logs", "Add problem", "Add step", "Launch test", "bad signature in batch, rejected at verify", "low bandwidth on path to main", "restart sidecar", "batch"} {
		if !bytes.Contains([]byte(body), []byte(label)) {
			t.Fatalf("page missing %s", label)
		}
	}
}

func TestSecondRunIsRefused(t *testing.T) {
	srv := &Server{}
	srv.busy = true
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/scenarios/happy", bytes.NewBufferString(`{"count":10,"threads":1}`))
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("status %d %s", rr.Code, rr.Body.String())
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte("already in progress")) {
		t.Fatal(rr.Body.String())
	}
}

func TestHappyScenarioReachesPass(t *testing.T) {
	spammer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"run_id":"run1"}`))
	}))
	defer spammer.Close()
	sinkSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(sink.Stats{
			RunID:       "run1",
			Sent:        4,
			Received:    4,
			Missing:     []int{},
			ByCollector: map[string]int{"main": 4, "secondary": 0},
			LatencyMS:   sink.Latency{Min: 1, Avg: 2, P50: 2, P95: 3, Max: 4},
			Status:      "drained",
		})
	}))
	defer sinkSrv.Close()
	uiSrv := &Server{SpammerURL: spammer.URL, SinkURL: sinkSrv.URL}
	rr := httptest.NewRecorder()
	uiSrv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/scenarios/happy", bytes.NewBufferString(`{"count":4,"threads":1}`)))
	if rr.Code != http.StatusAccepted {
		t.Fatal(rr.Body.String())
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		statsRR := httptest.NewRecorder()
		uiSrv.Handler().ServeHTTP(statsRR, httptest.NewRequest(http.MethodGet, "/api/stats?run_id=run1", nil))
		var got Result
		if err := json.Unmarshal(statsRR.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if got.Pass != nil && *got.Pass && got.Received == 4 && got.ByCollector["main"] == 4 {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatal("happy run did not pass")
}

func TestJudgements(t *testing.T) {
	ok, _ := judgeMalformed(sink.Stats{Sent: 10, Received: 5, Rejected: 5})
	if !ok {
		t.Fatal("malformed should pass")
	}
	ok, _ = judgeBadSignature(sink.Stats{Sent: 10, Received: 0, VerifyFailures: 10})
	if !ok {
		t.Fatal("bad signature should pass")
	}
	ok, _ = judgeAllSecondary(sink.Stats{Sent: 3, Received: 3, Missing: []int{}, ByCollector: map[string]int{"main": 0, "secondary": 3}})
	if !ok {
		t.Fatal("secondary should pass")
	}
	ok, _ = judgeAllMain(sink.Stats{Sent: 3, Received: 2, ByCollector: map[string]int{"main": 2}})
	if ok {
		t.Fatal("short receive should fail")
	}
}

func TestScenariosJSON(t *testing.T) {
	rr := httptest.NewRecorder()
	(&Server{}).Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/scenarios", nil))
	var list []Scenario
	if err := json.Unmarshal(rr.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 11 {
		t.Fatalf("got %d", len(list))
	}
	_, _ = io.ReadAll(rr.Body)
}
