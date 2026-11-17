package spammer

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"audit-loss/internal/otlpjson"
)

type Server struct {
	OTLPURL string
	Client  *http.Client
	mu      sync.Mutex
	running bool
	last    *Run
	gate    *release
}

type Run struct {
	RunID      string `json:"run_id"`
	Scenario   string `json:"scenario"`
	Sent       int    `json:"sent"`
	Accepted   int    `json:"accepted"`
	Rejected   int    `json:"rejected"`
	StartedAt  string `json:"started_at"`
	FinishedAt string `json:"finished_at"`
	Status     string `json:"status"`
	Fault      string `json:"fault,omitempty"`
	Holding    bool   `json:"holding"`
	HoldAt     int    `json:"hold_at"`
}

type Injection struct {
	Every int    `json:"every"`
	Fault string `json:"fault"`
}

type release struct {
	once sync.Once
	ch   chan struct{}
}

func (r *release) signal() {
	if r == nil {
		return
	}
	r.once.Do(func() { close(r.ch) })
}

type Request struct {
	Count           int         `json:"count"`
	Threads         int         `json:"threads"`
	Rate            int         `json:"rate"`
	PayloadBytes    int         `json:"payload_bytes"`
	BatchSize       int         `json:"batch_size"`
	ClientTimeoutMS int         `json:"client_timeout_ms"`
	Retry           bool        `json:"retry"`
	Scenario        string      `json:"scenario"`
	Fault           string      `json:"fault"`
	Every           int         `json:"every"`
	Injections      []Injection `json:"injections"`
	Gates           []int       `json:"gates"`
	RunID           string      `json:"run_id"`
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /last-run", s.lastRun)
	mux.HandleFunc("POST /run", s.start)
	mux.HandleFunc("POST /continue", s.continueRun)
	return mux
}

func (s *Server) lastRun(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.last == nil {
		writeJSON(w, http.StatusOK, Run{})
		return
	}
	writeJSON(w, http.StatusOK, *s.last)
}

func (s *Server) start(w http.ResponseWriter, r *http.Request) {
	var req Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.Count <= 0 {
		req.Count = 1
	}
	if req.Threads <= 0 {
		req.Threads = 1
	}
	if req.BatchSize <= 0 {
		req.BatchSize = 1
	}
	if req.Scenario == "" {
		req.Scenario = "manual"
	}
	if req.Fault == "" {
		req.Fault = "none"
	}
	if err := req.normalize(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.RunID == "" {
		req.RunID = newID()
	}
	if req.ClientTimeoutMS <= 0 {
		req.ClientTimeoutMS = 5000
	}

	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		http.Error(w, "a run is already in progress", http.StatusConflict)
		return
	}
	s.running = true
	run := &Run{
		RunID:     req.RunID,
		Scenario:  req.Scenario,
		StartedAt: time.Now().Format(time.RFC3339Nano),
		Status:    "running",
		Fault:     req.Fault,
	}
	s.last = run
	s.mu.Unlock()

	log.Printf("run %s scenario=%s count=%d fault=%s", run.RunID, run.Scenario, req.Count, req.Fault)
	go s.execute(req, run)
	writeJSON(w, http.StatusAccepted, run)
}

func (s *Server) execute(req Request, run *Run) {
	defer func() {
		s.mu.Lock()
		run.FinishedAt = time.Now().Format(time.RFC3339Nano)
		run.Status = "finished"
		s.running = false
		log.Printf("run %s finished sent=%d accepted=%d rejected=%d", run.RunID, run.Sent, run.Accepted, run.Rejected)
		s.mu.Unlock()
	}()

	seqs := make(chan int)
	var wg sync.WaitGroup
	for i := 0; i < req.Threads; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.worker(req, run, seqs)
		}()
	}
	gates := append([]int(nil), req.Gates...)
	next := 0
	for i := 1; i <= req.Count; i++ {
		s.pace(req.Rate, i)
		seqs <- i
		if next < len(gates) && i == gates[next] {
			s.waitSent(run, i)
			s.setHold(run, i)
			next++
		}
	}
	close(seqs)
	wg.Wait()
}

func (s *Server) pace(rate, i int) {
	if rate <= 0 || i == 1 {
		return
	}
	time.Sleep(time.Second / time.Duration(rate))
}

func (s *Server) worker(req Request, run *Run, seqs <-chan int) {
	batch := make([]int, 0, req.BatchSize)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		s.sendBatch(req, run, batch)
		batch = batch[:0]
	}
	for seq := range seqs {
		batch = append(batch, seq)
		if len(batch) >= req.BatchSize {
			flush()
		}
	}
	flush()
}

func (req *Request) normalize() error {
	if req.Every < 0 {
		return fmt.Errorf("every must be >= 0")
	}
	if len(req.Injections) == 0 && req.Every > 0 && req.Fault != "" && req.Fault != "none" {
		req.Injections = []Injection{{Every: req.Every, Fault: req.Fault}}
	}
	kept := req.Injections[:0]
	for _, inj := range req.Injections {
		if inj.Fault == "" || inj.Fault == "none" {
			continue
		}
		if inj.Every < 0 || !everyFaultOK(inj.Fault) {
			return fmt.Errorf("fault cannot be applied every N logs")
		}
		if inj.Every == 0 && inj.Fault != "bad_in_batch" {
			continue
		}
		kept = append(kept, inj)
	}
	req.Injections = kept
	gates := make([]int, 0, len(req.Gates))
	seen := map[int]bool{}
	for _, after := range req.Gates {
		if after <= 0 || after > req.Count || seen[after] {
			continue
		}
		seen[after] = true
		gates = append(gates, after)
	}
	sort.Ints(gates)
	req.Gates = gates
	if req.perLog() || len(req.Gates) > 0 {
		req.BatchSize = 1
	}
	return nil
}

func (req Request) perLog() bool {
	if len(req.Injections) == 0 {
		return req.Every > 0 && req.Fault != "" && req.Fault != "none" && req.Fault != "bad_in_batch"
	}
	for _, inj := range req.Injections {
		if inj.Fault != "bad_in_batch" {
			return true
		}
	}
	return false
}

func (s *Server) continueRun(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	rel := s.gate
	s.mu.Unlock()
	rel.signal()
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) waitSent(run *Run, n int) {
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		sent := run.Sent
		s.mu.Unlock()
		if sent >= n {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func (s *Server) setHold(run *Run, at int) {
	rel := &release{ch: make(chan struct{})}
	s.mu.Lock()
	run.Holding = true
	run.HoldAt = at
	s.gate = rel
	s.mu.Unlock()
	select {
	case <-rel.ch:
	case <-time.After(90 * time.Second):
		rel.signal()
	}
	s.mu.Lock()
	run.Holding = false
	if s.gate == rel {
		s.gate = nil
	}
	s.mu.Unlock()
}

func everyFaultOK(fault string) bool {
	switch fault {
	case "none", "malformed_json", "missing_attrs", "bad_timestamp", "oversize", "duplicate_id", "connection_reset", "bad_signature", "bad_in_batch", "stall", "slow":
		return true
	default:
		return false
	}
}

func (req Request) hits(seq int) bool {
	return req.faultAt(seq) != ""
}

func (req Request) badInBatch(seq int, seqs []int) bool {
	if req.faultAt(seq) == "bad_in_batch" {
		return true
	}
	if len(seqs) < 2 || seq != seqs[len(seqs)-1] {
		return false
	}
	for _, inj := range req.Injections {
		if inj.Fault == "bad_in_batch" && inj.Every == 0 {
			return true
		}
	}
	return req.Fault == "bad_in_batch" && req.Every == 0 && len(req.Injections) == 0
}

func (req Request) faultAt(seq int) string {
	if len(req.Injections) > 0 {
		for _, inj := range req.Injections {
			if inj.Every > 0 && seq%inj.Every == 0 {
				return inj.Fault
			}
		}
		return ""
	}
	if req.Every > 0 && req.Fault != "" && req.Fault != "none" && seq%req.Every == 0 {
		return req.Fault
	}
	return ""
}

func (s *Server) sendBatch(req Request, run *Run, seqs []int) {
	if req.perLog() {
		for _, seq := range seqs {
			s.sendOne(req, run, seq)
		}
		return
	}
	for range seqs {
		s.bump(run, func(run *Run) { run.Sent++ })
	}
	if req.Fault == "malformed_json" {
		var good []int
		for _, seq := range seqs {
			if seq%2 == 1 {
				s.postBytes(req, run, []byte(`{"not":`), false)
				continue
			}
			good = append(good, seq)
		}
		if len(good) == 0 {
			return
		}
		seqs = good
	}
	if req.Fault == "duplicate_id" && len(seqs) > 0 {
		seqs = append(seqs, seqs[0])
	}
	records := make([]otlpjson.Record, 0, len(seqs))
	for _, seq := range seqs {
		body := "audit log " + strconv.Itoa(seq)
		if req.PayloadBytes > 0 {
			body += strings.Repeat("x", req.PayloadBytes)
		}
		if req.Fault == "oversize" {
			body += strings.Repeat("x", 256<<10)
		}
		rec := otlpjson.Record{
			Sequence: strconv.Itoa(seq),
			Scenario: req.Scenario,
			RunID:    req.RunID,
			Emit:     strconv.FormatInt(time.Now().UnixNano(), 10),
			Body:     body,
			Attrs:    map[string]string{},
		}
		if req.badInBatch(seq, seqs) {
			rec.Attrs["demo.inject"] = "bad_signature"
		}
		if req.Fault == "missing_attrs" {
			rec.Sequence = ""
		}
		if req.Fault == "bad_timestamp" {
			rec.Emit = "not-a-time"
		}
		records = append(records, rec)
	}
	raw, err := otlpjson.Build(records)
	if err != nil {
		s.bump(run, func(run *Run) { run.Rejected += len(seqs) })
		return
	}
	switch req.Fault {
	case "die_after_send", "connection_reset":
		_ = writeAndClose(s.OTLPURL, raw)
		if req.Fault == "die_after_send" {
			os.Exit(0)
		}
		return
	}
	s.postBytes(req, run, raw, true)
}

func (s *Server) sendOne(req Request, run *Run, seq int) {
	s.bump(run, func(run *Run) { run.Sent++ })
	fault := req.faultAt(seq)
	if fault == "malformed_json" {
		s.postBytes(req, run, []byte(`{"not":`), false)
		return
	}
	if fault == "connection_reset" {
		raw, err := s.oneRecord(req, seq, false)
		if err != nil {
			s.bump(run, func(run *Run) { run.Rejected++ })
			return
		}
		_ = writeAndClose(s.OTLPURL, raw)
		return
	}
	raw, err := s.oneRecord(req, seq, true)
	if err != nil {
		s.bump(run, func(run *Run) { run.Rejected++ })
		return
	}
	s.postBytes(req, run, raw, true)
	if fault == "duplicate_id" {
		s.postBytes(req, run, raw, true)
	}
}

func (s *Server) oneRecord(req Request, seq int, mark bool) ([]byte, error) {
	body := "audit log " + strconv.Itoa(seq)
	if req.PayloadBytes > 0 {
		body += strings.Repeat("x", req.PayloadBytes)
	}
	rec := otlpjson.Record{
		Sequence: strconv.Itoa(seq),
		Scenario: req.Scenario,
		RunID:    req.RunID,
		Emit:     strconv.FormatInt(time.Now().UnixNano(), 10),
		Body:     body,
		Attrs:    map[string]string{},
	}
	if mark {
		switch req.faultAt(seq) {
		case "oversize":
			rec.Body += strings.Repeat("x", 256<<10)
		case "missing_attrs":
			rec.Sequence = ""
		case "bad_timestamp":
			rec.Emit = "not-a-time"
		case "bad_signature", "bad_in_batch", "stall", "slow":
			name := req.faultAt(seq)
			if name == "bad_in_batch" {
				name = "bad_signature"
			}
			rec.Attrs["demo.inject"] = name
		}
	}
	return otlpjson.Build([]otlpjson.Record{rec})
}

func (s *Server) postBytes(req Request, run *Run, raw []byte, countResult bool) {
	client := s.Client
	if client == nil {
		client = &http.Client{Timeout: time.Duration(req.ClientTimeoutMS) * time.Millisecond}
	}
	attempts := 1
	if req.Retry {
		attempts = 2
	}
	var lastCode int
	var lastErr error
	for i := 0; i < attempts; i++ {
		httpReq, err := http.NewRequest(http.MethodPost, s.OTLPURL, bytes.NewReader(raw))
		if err != nil {
			lastErr = err
			break
		}
		httpReq.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(httpReq)
		if err != nil {
			lastErr = err
			continue
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		lastCode = resp.StatusCode
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			if countResult {
				s.bump(run, func(run *Run) { run.Accepted += acceptedCount(req, raw) })
			}
			return
		}
	}
	if countResult {
		n := acceptedCount(req, raw)
		s.bump(run, func(run *Run) { run.Rejected += n })
	} else {
		s.bump(run, func(run *Run) { run.Rejected++ })
	}
	if lastErr != nil {
		log.Printf("run %s post failed: %v", run.RunID, lastErr)
	} else if lastCode != 0 {
		log.Printf("run %s post status %d", run.RunID, lastCode)
	}
}

func acceptedCount(req Request, raw []byte) int {
	if req.Fault == "malformed_json" && bytes.Equal(raw, []byte(`{"not":`)) {
		return 1
	}
	doc, err := otlpjson.Parse(raw)
	if err != nil {
		return 1
	}
	n := 0
	otlpjson.Walk(doc, func(map[string]any) { n++ })
	if n == 0 {
		return 1
	}
	return n
}

func (s *Server) bump(run *Run, fn func(*Run)) {
	s.mu.Lock()
	fn(run)
	s.mu.Unlock()
}

func writeAndClose(rawURL string, body []byte) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return err
	}
	host := u.Host
	if !strings.Contains(host, ":") {
		host += ":80"
	}
	conn, err := net.DialTimeout("tcp", host, 2*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()
	path := u.RequestURI()
	if path == "" {
		path = "/v1/logs"
	}
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "POST %s HTTP/1.1\r\nHost: %s\r\nContent-Type: application/json\r\nContent-Length: %d\r\nConnection: close\r\n\r\n", path, u.Host, len(body))
	buf.Write(body)
	_, err = conn.Write(buf.Bytes())
	return err
}

func newID() string {
	var b [6]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}
