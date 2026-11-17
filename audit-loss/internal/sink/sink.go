package sink

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"audit-loss/internal/otlpjson"
)

type Store struct {
	Dir        string
	SpammerURL string
	DrainFor   time.Duration
	Client     *http.Client
	Delay      func() time.Duration
	After      func() time.Duration
	now        func() time.Time
	mu         sync.Mutex
	runs       map[string]*runState
}

type runState struct {
	collectors map[int]string
	latency    map[int]float64
	duplicates int
	verifyFail int
	sent       int
	rejected   int
	started    time.Time
	finished   time.Time
	lastRecv   time.Time
	loaded     bool
}

type Stats struct {
	RunID          string         `json:"run_id"`
	Sent           int            `json:"sent"`
	Received       int            `json:"received"`
	Missing        []int          `json:"missing"`
	Duplicates     int            `json:"duplicates"`
	ByCollector    map[string]int `json:"by_collector"`
	VerifyFailures int            `json:"verify_failures"`
	Rejected       int            `json:"rejected"`
	LatencyMS      Latency        `json:"latency_ms"`
	StartedAt      string         `json:"started_at"`
	FinishedAt     string         `json:"finished_at"`
	DrainedAt      string         `json:"drained_at"`
	Status         string         `json:"status"`
}

type Latency struct {
	Min float64 `json:"min"`
	Avg float64 `json:"avg"`
	P50 float64 `json:"p50"`
	P95 float64 `json:"p95"`
	Max float64 `json:"max"`
}

type spammerSnapshot struct {
	RunID      string `json:"run_id"`
	Sent       int    `json:"sent"`
	Rejected   int    `json:"rejected"`
	StartedAt  string `json:"started_at"`
	FinishedAt string `json:"finished_at"`
}

func New(dir, spammerURL string, drain time.Duration) *Store {
	return &Store{
		Dir:        dir,
		SpammerURL: spammerURL,
		DrainFor:   drain,
		Client:     http.DefaultClient,
		now:        time.Now,
		runs:       map[string]*runState{},
	}
}

func (s *Store) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("POST /v1/logs", s.ingest)
	mux.HandleFunc("POST /rejected", s.rejected)
	mux.HandleFunc("GET /stats", s.stats)
	mux.HandleFunc("POST /admin/delay", s.delay)
	return mux
}

func (s *Store) ingest(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 8<<20))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if s.Delay != nil {
		if d := s.Delay(); d > 0 {
			time.Sleep(d)
		}
	}
	doc, err := otlpjson.Parse(body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	slow, stall := injectHolds(doc)
	if slow {
		time.Sleep(400 * time.Millisecond)
	}
	recv := s.now()
	otlpjson.Walk(doc, func(rec map[string]any) {
		seqText := otlpjson.GetAttr(rec, "demo.sequence")
		seq, err := strconv.Atoi(seqText)
		if err != nil || seq <= 0 {
			return
		}
		runID := otlpjson.GetAttr(rec, "demo.run_id")
		if runID == "" {
			return
		}
		emitText := otlpjson.GetAttr(rec, "demo.emit_time_unix_nano")
		latency := latencyMS(emitText, recv)
		collector := otlpjson.GetAttr(rec, "demo.collector")
		if collector == "" {
			collector = "unknown"
		}
		s.save(runID, seq, collector, latency, recordText(seq, otlpjson.GetAttr(rec, "demo.scenario"), collector, emitText, recv, latency, otlpjson.Body(rec)))
	})
	if s.After != nil {
		if d := s.After(); d > 0 {
			time.Sleep(d)
		}
	}
	if stall {
		time.Sleep(6 * time.Second)
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func injectHolds(doc map[string]any) (slow bool, stall bool) {
	otlpjson.Walk(doc, func(rec map[string]any) {
		switch otlpjson.GetAttr(rec, "demo.inject") {
		case "slow":
			slow = true
		case "stall":
			stall = true
		}
	})
	return slow, stall
}

func (s *Store) save(runID string, seq int, collector string, latency float64, text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.state(runID)
	path := filepath.Join(s.Dir, runID, strconv.Itoa(seq)+".txt")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		st.duplicates++
		_ = s.writeMeta(runID, st)
		return
	}
	_, _ = f.WriteString(text)
	_ = f.Sync()
	_ = f.Close()
	st.collectors[seq] = collector
	st.latency[seq] = latency
	st.lastRecv = s.now()
}

func (s *Store) rejected(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RunID    string `json:"run_id"`
		Sequence int    `json:"sequence"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.RunID == "" {
		http.Error(w, "bad rejection", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	st := s.state(body.RunID)
	st.verifyFail++
	_ = s.writeMeta(body.RunID, st)
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Store) stats(w http.ResponseWriter, r *http.Request) {
	runID := r.URL.Query().Get("run_id")
	if runID == "" {
		http.Error(w, "run_id is required", http.StatusBadRequest)
		return
	}
	s.refreshSpammer(runID)
	s.mu.Lock()
	st := s.state(runID)
	out := s.compute(runID, st)
	s.mu.Unlock()
	raw, _ := json.MarshalIndent(out, "", "  ")
	_ = os.MkdirAll(filepath.Join(s.Dir, runID), 0o755)
	_ = os.WriteFile(filepath.Join(s.Dir, runID, "summary.json"), raw, 0o644)
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(append(raw, '\n'))
}

func (s *Store) delay(w http.ResponseWriter, r *http.Request) {
	var body struct {
		MS    int  `json:"ms"`
		After bool `json:"after"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.MS < 0 {
		http.Error(w, "bad delay", http.StatusBadRequest)
		return
	}
	ms := body.MS
	hold := func() time.Duration { return time.Duration(ms) * time.Millisecond }
	none := func() time.Duration { return 0 }
	if body.After {
		s.After = hold
		s.Delay = none
	} else {
		s.Delay = hold
		s.After = none
	}
	writeJSON(w, http.StatusOK, map[string]any{"ms": ms, "after": body.After})
}

func (s *Store) refreshSpammer(runID string) {
	if s.SpammerURL == "" {
		return
	}
	req, err := http.NewRequest(http.MethodGet, strings.TrimRight(s.SpammerURL, "/")+"/last-run", nil)
	if err != nil {
		return
	}
	client := s.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	var snap spammerSnapshot
	if err := json.NewDecoder(resp.Body).Decode(&snap); err != nil || snap.RunID != runID {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.state(runID)
	st.sent = snap.Sent
	st.rejected = snap.Rejected
	if t, err := time.Parse(time.RFC3339Nano, snap.StartedAt); err == nil {
		st.started = t
	}
	if snap.FinishedAt != "" {
		if t, err := time.Parse(time.RFC3339Nano, snap.FinishedAt); err == nil {
			st.finished = t
		}
	}
}

func (s *Store) compute(runID string, st *runState) Stats {
	by := map[string]int{"main": 0, "secondary": 0}
	lats := make([]float64, 0, len(st.latency))
	for seq, collector := range st.collectors {
		by[collector]++
		lats = append(lats, st.latency[seq])
	}
	sort.Float64s(lats)
	missing := []int{}
	if !st.finished.IsZero() && s.drained(st) {
		have := map[int]struct{}{}
		for seq := range st.collectors {
			have[seq] = struct{}{}
		}
		for i := 1; i <= st.sent; i++ {
			if _, ok := have[i]; !ok {
				missing = append(missing, i)
			}
		}
	}
	status := "running"
	drainedAt := ""
	if s.drained(st) {
		status = "drained"
		anchor := st.finished
		if st.lastRecv.After(anchor) {
			anchor = st.lastRecv
		}
		drainedAt = anchor.Add(s.DrainFor).Format(time.RFC3339Nano)
	}
	return Stats{
		RunID:          runID,
		Sent:           st.sent,
		Received:       len(st.collectors),
		Missing:        missing,
		Duplicates:     st.duplicates,
		ByCollector:    by,
		VerifyFailures: st.verifyFail,
		Rejected:       st.rejected,
		LatencyMS:      latencyStats(lats),
		StartedAt:      formatTime(st.started),
		FinishedAt:     formatTime(st.finished),
		DrainedAt:      drainedAt,
		Status:         status,
	}
}

func (s *Store) drained(st *runState) bool {
	if st.finished.IsZero() {
		return false
	}
	anchor := st.finished
	if st.lastRecv.After(anchor) {
		anchor = st.lastRecv
	}
	return !s.now().Before(anchor.Add(s.DrainFor))
}

func (s *Store) state(runID string) *runState {
	if st, ok := s.runs[runID]; ok {
		return st
	}
	st := &runState{
		collectors: map[int]string{},
		latency:    map[int]float64{},
	}
	s.load(runID, st)
	s.runs[runID] = st
	return st
}

func (s *Store) load(runID string, st *runState) {
	dir := filepath.Join(s.Dir, runID)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".txt") {
			continue
		}
		seq, err := strconv.Atoi(strings.TrimSuffix(name, ".txt"))
		if err != nil {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		fields := parseFields(raw)
		st.collectors[seq] = fields["collector"]
		if lat, err := strconv.ParseFloat(fields["latency_ms"], 64); err == nil {
			st.latency[seq] = lat
		}
	}
	metaRaw, err := os.ReadFile(filepath.Join(dir, "meta.json"))
	if err != nil {
		return
	}
	var meta struct {
		Duplicates int `json:"duplicates"`
		VerifyFail int `json:"verify_failures"`
	}
	if json.Unmarshal(metaRaw, &meta) == nil {
		st.duplicates = meta.Duplicates
		st.verifyFail = meta.VerifyFail
	}
}

func (s *Store) writeMeta(runID string, st *runState) error {
	raw, err := json.Marshal(map[string]int{
		"duplicates":      st.duplicates,
		"verify_failures": st.verifyFail,
	})
	if err != nil {
		return err
	}
	dir := filepath.Join(s.Dir, runID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "meta.json"), raw, 0o644)
}

func recordText(seq int, scenario, collector, emit string, recv time.Time, latency float64, body string) string {
	return fmt.Sprintf("sequence=%d\nscenario=%s\ncollector=%s\nemit_unix_nano=%s\nreceive_unix_nano=%d\nlatency_ms=%s\nbody=%s\n",
		seq, scenario, collector, emit, recv.UnixNano(), strconv.FormatFloat(latency, 'f', 3, 64), body)
}

func parseFields(raw []byte) map[string]string {
	out := map[string]string{}
	sc := bufio.NewScanner(bytes.NewReader(raw))
	for sc.Scan() {
		line := sc.Text()
		k, v, ok := strings.Cut(line, "=")
		if ok {
			out[k] = v
		}
	}
	return out
}

func latencyMS(emit string, recv time.Time) float64 {
	n, err := strconv.ParseInt(emit, 10, 64)
	if err != nil {
		return 0
	}
	delta := float64(recv.UnixNano()-n) / 1e6
	return delta
}

func latencyStats(sorted []float64) Latency {
	if len(sorted) == 0 {
		return Latency{}
	}
	sum := 0.0
	for _, v := range sorted {
		sum += v
	}
	return Latency{
		Min: sorted[0],
		Avg: sum / float64(len(sorted)),
		P50: percentile(sorted, 50),
		P95: percentile(sorted, 95),
		Max: sorted[len(sorted)-1],
	}
}

func percentile(sorted []float64, p float64) float64 {
	rank := int(math.Ceil(p/100*float64(len(sorted)))) - 1
	if rank < 0 {
		rank = 0
	}
	if rank >= len(sorted) {
		rank = len(sorted) - 1
	}
	return sorted[rank]
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339Nano)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}
