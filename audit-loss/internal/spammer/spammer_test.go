package spammer

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"audit-loss/internal/otlpjson"
)

func TestHappyRunAndMalformedSplit(t *testing.T) {
	var good, bad int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if !json.Valid(body) {
			bad++
			http.Error(w, "bad", http.StatusBadRequest)
			return
		}
		doc, err := otlpjson.Parse(body)
		if err != nil {
			t.Errorf("parse %v", err)
			http.Error(w, "bad", http.StatusBadRequest)
			return
		}
		otlpjson.Walk(doc, func(map[string]any) { good++ })
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	srv := &Server{OTLPURL: upstream.URL + "/v1/logs", Client: upstream.Client()}
	start := httptest.NewRequest(http.MethodPost, "/run", bytes.NewBufferString(`{"count":4,"batch_size":2,"scenario":"happy","run_id":"r1"}`))
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, start)
	if rr.Code != http.StatusAccepted {
		t.Fatal(rr.Body.String())
	}
	run := waitRun(t, srv, "r1")
	if run.Sent != 4 || run.Accepted != 4 || run.Rejected != 0 || good != 4 {
		t.Fatalf("run %+v good %d bad %d", run, good, bad)
	}

	good, bad = 0, 0
	start = httptest.NewRequest(http.MethodPost, "/run", bytes.NewBufferString(`{"count":4,"batch_size":4,"scenario":"malformed","fault":"malformed_json","run_id":"r2"}`))
	rr = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, start)
	run = waitRun(t, srv, "r2")
	if run.Sent != 4 || run.Accepted != 2 || run.Rejected != 2 || good != 2 || bad != 2 {
		t.Fatalf("malformed %+v good %d bad %d", run, good, bad)
	}
}

func TestEverySecondLogIsMalformed(t *testing.T) {
	var good, bad int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if !json.Valid(body) {
			bad++
			http.Error(w, "bad", http.StatusBadRequest)
			return
		}
		good++
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()
	srv := &Server{OTLPURL: upstream.URL + "/v1/logs", Client: upstream.Client()}
	req := httptest.NewRequest(http.MethodPost, "/run", bytes.NewBufferString(`{"count":4,"scenario":"custom","fault":"malformed_json","every":2,"run_id":"r3"}`))
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatal(rr.Body.String())
	}
	run := waitRun(t, srv, "r3")
	if run.Sent != 4 || run.Accepted != 2 || run.Rejected != 2 || good != 2 || bad != 2 {
		t.Fatalf("every %+v good %d bad %d", run, good, bad)
	}
}

func TestTwoInjectionsUseTheFirstMatch(t *testing.T) {
	var malformed, slow, plain int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if !json.Valid(body) {
			malformed++
			http.Error(w, "bad", http.StatusBadRequest)
			return
		}
		doc, err := otlpjson.Parse(body)
		if err != nil {
			t.Errorf("parse %v", err)
			http.Error(w, "bad", http.StatusBadRequest)
			return
		}
		otlpjson.Walk(doc, func(rec map[string]any) {
			if otlpjson.GetAttr(rec, "demo.inject") == "slow" {
				slow++
				return
			}
			plain++
		})
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()
	srv := &Server{OTLPURL: upstream.URL + "/v1/logs", Client: upstream.Client()}
	raw := `{"count":6,"run_id":"r4","injections":[{"every":2,"fault":"malformed_json"},{"every":3,"fault":"slow"}]}`
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/run", bytes.NewBufferString(raw)))
	if rr.Code != http.StatusAccepted {
		t.Fatal(rr.Body.String())
	}
	run := waitRun(t, srv, "r4")
	if run.Sent != 6 || malformed != 3 || slow != 1 || plain != 2 {
		t.Fatalf("run %+v malformed %d slow %d plain %d", run, malformed, slow, plain)
	}
}

func TestOneBadLogStaysInsideTheBatch(t *testing.T) {
	var requests int
	var badAt []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		body, _ := io.ReadAll(r.Body)
		doc, err := otlpjson.Parse(body)
		if err != nil {
			t.Errorf("parse %v", err)
			http.Error(w, "bad", http.StatusBadRequest)
			return
		}
		otlpjson.Walk(doc, func(rec map[string]any) {
			if otlpjson.GetAttr(rec, "demo.inject") == "bad_signature" {
				badAt = append(badAt, otlpjson.GetAttr(rec, "demo.sequence"))
			}
		})
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()
	srv := &Server{OTLPURL: upstream.URL + "/v1/logs", Client: upstream.Client()}
	raw := `{"count":4,"batch_size":4,"run_id":"r6","injections":[{"every":0,"fault":"bad_in_batch"}]}`
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/run", bytes.NewBufferString(raw)))
	if rr.Code != http.StatusAccepted {
		t.Fatal(rr.Body.String())
	}
	run := waitRun(t, srv, "r6")
	if run.Sent != 4 || requests != 1 || len(badAt) != 1 || badAt[0] != "4" {
		t.Fatalf("run %+v requests %d bad %v", run, requests, badAt)
	}
}

func TestGateHoldsUntilContinue(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()
	srv := &Server{OTLPURL: upstream.URL + "/v1/logs", Client: upstream.Client()}
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/run", bytes.NewBufferString(`{"count":4,"gates":[2],"run_id":"r5"}`)))
	if rr.Code != http.StatusAccepted {
		t.Fatal(rr.Body.String())
	}
	deadline := time.Now().Add(3 * time.Second)
	held := false
	for time.Now().Before(deadline) {
		snap := httptest.NewRecorder()
		srv.Handler().ServeHTTP(snap, httptest.NewRequest(http.MethodGet, "/last-run", nil))
		var run Run
		if err := json.Unmarshal(snap.Body.Bytes(), &run); err != nil {
			t.Fatal(err)
		}
		if run.Holding && run.HoldAt == 2 && run.Sent >= 2 {
			held = true
			break
		}
		if run.Status == "finished" {
			t.Fatal("finished before the gate")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !held {
		t.Fatal("gate did not hold")
	}
	cont := httptest.NewRecorder()
	srv.Handler().ServeHTTP(cont, httptest.NewRequest(http.MethodPost, "/continue", nil))
	if cont.Code != http.StatusOK {
		t.Fatal(cont.Body.String())
	}
	run := waitRun(t, srv, "r5")
	if run.Sent != 4 || run.Holding {
		t.Fatalf("after continue %+v", run)
	}
}

func waitRun(t *testing.T, srv *Server, id string) Run {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		rr := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/last-run", nil))
		var run Run
		if err := json.Unmarshal(rr.Body.Bytes(), &run); err != nil {
			t.Fatal(err)
		}
		if run.RunID == id && run.Status == "finished" {
			return run
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("run did not finish")
	return Run{}
}
