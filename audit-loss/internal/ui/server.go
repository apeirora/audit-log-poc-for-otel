package ui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"audit-loss/internal/sink"
)

type Scenario struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Group    string `json:"group"`
	Expected string `json:"expected"`
}

type Result struct {
	sink.Stats
	Pass     *bool  `json:"pass"`
	Expected string `json:"expected"`
	Detail   string `json:"detail"`
	Error    string `json:"error,omitempty"`
}

type Server struct {
	SpammerURL      string
	ToxicURL        string
	SinkURL         string
	Main            string
	Secondary       string
	Spammer         string
	Sidecar         string
	MainHealth      string
	SecondaryHealth string
	SidecarHealth   string
	Docker          interface {
		Stop(name string) error
		Start(name string) error
		Kill(name string) error
		Running(name string) (bool, error)
	}
	Client *http.Client

	mu      sync.Mutex
	busy    bool
	current Result
}

func Scenarios() []Scenario {
	return []Scenario{
		{ID: "happy", Label: "Happy path", Group: "delivery", Expected: "Every log arrives through the main collector."},
		{ID: "slow-sink", Label: "Slow on sink", Group: "delivery", Expected: "Every log arrives. Latency is higher because the sink waits before it accepts."},
		{ID: "stall", Label: "Stall on sink", Group: "delivery", Expected: "The sink holds the response. The exporter retries, so the unique count matches sent and duplicates is at least one."},
		{ID: "main-down", Label: "Main down", Group: "failover", Expected: "Main is stopped. Every log arrives through the secondary collector. Main is started again at the end."},
		{ID: "main-unreachable", Label: "Main unreachable", Group: "failover", Expected: "The path to main is a blackhole. Every log arrives through the secondary collector."},
		{ID: "secondary-down", Label: "Secondary down", Group: "failover", Expected: "Secondary is stopped. Every log still arrives through main. Secondary is started again at the end."},
		{ID: "both-down", Label: "Both down", Group: "failover", Expected: "Both collectors are stopped, then started again. The sidecar queue holds the logs and they all arrive."},
		{ID: "malformed", Label: "Malformed logs from app", Group: "loss", Expected: "Odd sequences are broken JSON from the app. The signer rejects them before the sidecar. The well-formed ones arrive."},
		{ID: "bad-signature", Label: "Bad signature to main", Group: "loss", Expected: "The signature is flipped on the path to main. Verify on main rejects the logs, so received stays below sent."},
		{ID: "oom", Label: "Kill main collector", Group: "loss", Expected: "The main collector is killed before the burst. New logs arrive through the secondary collector. Main is started again at the end."},
		{ID: "app-dies", Label: "App dies mid-send", Group: "loss", Expected: "The app exits after it writes the request and before it reads the ack. The spammer is started again afterward."},
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /", s.page)
	mux.HandleFunc("GET /api/scenarios", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, Scenarios())
	})
	mux.HandleFunc("POST /api/scenarios/{name}", s.start)
	mux.HandleFunc("GET /api/stats", s.stats)
	return mux
}

func (s *Server) page(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(pageHTML)
}

func (s *Server) start(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	sc, ok := scenarioByID(name)
	if !ok {
		http.Error(w, "unknown scenario", http.StatusNotFound)
		return
	}
	var body struct {
		Count      int         `json:"count"`
		Threads    int         `json:"threads"`
		Every      int         `json:"every"`
		Fault      string      `json:"fault"`
		Injections []injection `json:"injections"`
		Steps      []runStep   `json:"steps"`
		Batch      int         `json:"batch"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if body.Count <= 0 {
		body.Count = 100
	}
	if body.Threads <= 0 {
		body.Threads = 1
	}
	if body.Every < 0 {
		http.Error(w, "every must be >= 0", http.StatusBadRequest)
		return
	}
	if body.Batch < 0 {
		http.Error(w, "batch must be >= 0", http.StatusBadRequest)
		return
	}
	injections := body.Injections
	if len(injections) == 0 && body.Every > 0 && body.Fault != "" && body.Fault != "none" {
		injections = []injection{{Every: body.Every, Fault: body.Fault}}
	}
	kept := injections[:0]
	for _, inj := range injections {
		if inj.Fault == "" || inj.Fault == "none" {
			continue
		}
		if inj.Every < 0 || (inj.Every == 0 && inj.Fault != "bad_in_batch") {
			continue
		}
		if !faultOK(inj.Fault) {
			http.Error(w, "unknown fault "+inj.Fault, http.StatusBadRequest)
			return
		}
		kept = append(kept, inj)
	}
	steps := make([]runStep, 0, len(body.Steps))
	for _, step := range body.Steps {
		if step.After <= 0 || step.Action == "" || step.Action == "none" {
			continue
		}
		if step.After > body.Count {
			http.Error(w, "step is after the last log", http.StatusBadRequest)
			return
		}
		if !actionOK(step.Action) {
			http.Error(w, "unknown action "+step.Action, http.StatusBadRequest)
			return
		}
		steps = append(steps, step)
	}
	sort.Slice(steps, func(i, j int) bool { return steps[i].After < steps[j].After })
	sc.injections = kept
	sc.steps = steps
	sc.batch = body.Batch
	if len(sc.injections) > 0 || len(sc.steps) > 0 {
		sc.Expected = describe(sc.injections, sc.steps)
	}
	s.mu.Lock()
	if s.busy {
		s.mu.Unlock()
		writeJSON(w, http.StatusConflict, map[string]string{"error": "a run is already in progress"})
		return
	}
	s.busy = true
	s.current = Result{Expected: sc.Expected, Stats: sink.Stats{Status: "running"}}
	s.mu.Unlock()

	runID, err := s.prepareAndSend(sc, body.Count, body.Threads)
	if err != nil {
		s.mu.Lock()
		s.busy = false
		s.current.Error = err.Error()
		s.current.Status = "drained"
		pass := false
		s.current.Pass = &pass
		s.current.Detail = err.Error()
		s.mu.Unlock()
		_ = sc.cleanup(s)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	s.mu.Lock()
	s.current.RunID = runID
	s.mu.Unlock()
	go s.finish(sc, runID)
	writeJSON(w, http.StatusAccepted, map[string]string{"run_id": runID})
}

func (s *Server) stats(w http.ResponseWriter, r *http.Request) {
	runID := r.URL.Query().Get("run_id")
	s.mu.Lock()
	current := s.current
	s.mu.Unlock()
	if runID != "" && current.RunID == runID && current.Pass != nil {
		writeJSON(w, http.StatusOK, current)
		return
	}
	if runID == "" {
		writeJSON(w, http.StatusOK, current)
		return
	}
	st, err := s.sinkStats(runID)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	out := Result{Stats: st, Expected: current.Expected}
	if current.RunID == runID {
		out.Detail = current.Detail
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) finish(sc scenario, runID string) {
	defer func() {
		_ = sc.cleanup(s)
		s.mu.Lock()
		s.busy = false
		s.mu.Unlock()
	}()
	if len(sc.steps) > 0 && sc.ID != "app-dies" {
		s.playSteps(sc.steps, runID)
		s.waitSpammer(runID)
		s.restoreSteps(sc.steps)
	}
	if sc.ID == "both-down" {
		_ = s.Docker.Start(s.Main)
		_ = s.Docker.Start(s.Secondary)
		s.waitURL(s.MainHealth)
		s.waitURL(s.SecondaryHealth)
	}
	var st sink.Stats
	var err error
	if sc.ID == "app-dies" {
		st, err = s.waitApp(runID)
	} else if sc.ID == "stall" {
		time.Sleep(8 * time.Second)
		_ = s.sinkDelay(0, false)
		st, err = s.waitStable(runID, 70*time.Second)
	} else {
		st, err = s.waitStable(runID, 90*time.Second)
	}
	pass := false
	detail := ""
	if err != nil {
		detail = err.Error()
	} else if sc.ID == "app-dies" {
		pass = true
		detail = counts(st) + ". The spammer container was started again."
	} else if len(sc.injections) > 0 || len(sc.steps) > 0 {
		pass = true
		detail = describe(sc.injections, sc.steps) + counts(st)
	} else {
		pass, detail = sc.judge(st)
	}
	s.mu.Lock()
	s.current = Result{Stats: st, Pass: &pass, Expected: sc.Expected, Detail: detail}
	s.current.RunID = runID
	if s.current.Status == "" {
		s.current.Status = "drained"
	}
	s.mu.Unlock()
}

func (s *Server) prepareAndSend(sc scenario, count, threads int) (string, error) {
	if err := sc.prepare(s); err != nil {
		return "", err
	}
	fault := "none"
	batch := count
	if sc.batch > 0 && sc.batch < batch {
		batch = sc.batch
	}
	gates := make([]int, 0, len(sc.steps))
	for _, step := range sc.steps {
		gates = append(gates, step.After)
	}
	if perLog(sc.injections) || len(gates) > 0 {
		batch = 1
	} else if len(sc.injections) == 0 {
		if sc.ID == "malformed" {
			fault = "malformed_json"
		}
		if sc.ID == "app-dies" {
			fault = "die_after_send"
		}
	}
	payload := map[string]any{
		"count":      count,
		"threads":    threads,
		"batch_size": batch,
		"scenario":   sc.ID,
		"fault":      fault,
		"injections": sc.injections,
		"gates":      gates,
	}
	raw, _ := json.Marshal(payload)
	resp, err := s.client().Post(s.SpammerURL+"/run", "application/json", bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("spammer: %s %s", resp.Status, stringsTrim(body))
	}
	var run struct {
		RunID string `json:"run_id"`
	}
	if err := json.Unmarshal(body, &run); err != nil || run.RunID == "" {
		return "", fmt.Errorf("spammer returned no run id")
	}
	return run.RunID, nil
}

func (s *Server) waitStable(runID string, timeout time.Duration) (sink.Stats, error) {
	deadline := time.Now().Add(timeout)
	var prev sink.Stats
	stable := 0
	var last sink.Stats
	for time.Now().Before(deadline) {
		st, err := s.sinkStats(runID)
		if err != nil {
			return last, err
		}
		last = st
		if st.Status == "drained" && st.Received == prev.Received && st.Sent == prev.Sent && prev.Status == "drained" {
			stable++
			if stable >= 2 {
				return st, nil
			}
		} else {
			stable = 0
		}
		prev = st
		time.Sleep(time.Second)
	}
	return last, fmt.Errorf("timed out waiting for %s", runID)
}

func (s *Server) waitApp(runID string) (sink.Stats, error) {
	deadline := time.Now().Add(20 * time.Second)
	exited := false
	for time.Now().Before(deadline) {
		running, err := s.Docker.Running(s.Spammer)
		if err == nil && !running {
			exited = true
			break
		}
		time.Sleep(300 * time.Millisecond)
	}
	if !exited {
		return sink.Stats{}, fmt.Errorf("spammer did not exit")
	}
	if err := s.Docker.Start(s.Spammer); err != nil {
		return sink.Stats{}, err
	}
	time.Sleep(2 * time.Second)
	st, err := s.sinkStats(runID)
	if err != nil {
		return sink.Stats{Status: "drained"}, nil
	}
	st.Status = "drained"
	return st, nil
}

func (s *Server) waitURL(url string) {
	if url == "" {
		return
	}
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := s.client().Get(url)
		if err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(time.Second)
	}
}

func (s *Server) sinkStats(runID string) (sink.Stats, error) {
	resp, err := s.client().Get(s.SinkURL + "/stats?run_id=" + runID)
	if err != nil {
		return sink.Stats{}, err
	}
	defer resp.Body.Close()
	var st sink.Stats
	if err := json.NewDecoder(resp.Body).Decode(&st); err != nil {
		return sink.Stats{}, err
	}
	return st, nil
}

func (s *Server) sinkDelay(ms int, after bool) error {
	raw, _ := json.Marshal(map[string]any{"ms": ms, "after": after})
	resp, err := s.client().Post(s.SinkURL+"/admin/delay", "application/json", bytes.NewReader(raw))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("sink delay: %s", resp.Status)
	}
	return nil
}

func (s *Server) toxic(method, link string, fault any) error {
	var body io.Reader
	if fault != nil {
		raw, _ := json.Marshal(fault)
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, s.ToxicURL+"/faults/"+link, body)
	if err != nil {
		return err
	}
	if fault != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := s.client().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("toxic %s %s: %s", method, link, resp.Status)
	}
	return nil
}

func (s *Server) client() *http.Client {
	if s.Client != nil {
		return s.Client
	}
	s.Client = &http.Client{Timeout: 15 * time.Second}
	return s.Client
}

func scenarioByID(id string) (scenario, bool) {
	for _, sc := range catalog() {
		if sc.ID == id {
			return sc, true
		}
	}
	return scenario{}, false
}

type injection struct {
	Every int    `json:"every"`
	Fault string `json:"fault"`
}

type runStep struct {
	After  int    `json:"after"`
	Action string `json:"action"`
}

type spammerSnap struct {
	RunID   string `json:"run_id"`
	Sent    int    `json:"sent"`
	Status  string `json:"status"`
	Holding bool   `json:"holding"`
	HoldAt  int    `json:"hold_at"`
}

func perLog(injections []injection) bool {
	for _, inj := range injections {
		if inj.Fault != "bad_in_batch" {
			return true
		}
	}
	return false
}

func faultOK(fault string) bool {
	switch fault {
	case "malformed_json", "missing_attrs", "bad_timestamp", "oversize", "duplicate_id", "connection_reset", "bad_signature", "bad_in_batch", "stall", "slow":
		return true
	default:
		return false
	}
}

func actionOK(action string) bool {
	switch action {
	case "kill-main", "stop-main", "start-main", "kill-secondary", "stop-secondary", "start-secondary", "blackhole-main", "blackhole-secondary", "clear-faults", "low-bandwidth-main", "low-bandwidth-secondary", "low-bandwidth-sink", "restart-sidecar":
		return true
	default:
		return false
	}
}

func faultLabel(fault string) string {
	switch fault {
	case "malformed_json":
		return "malformed JSON from app"
	case "missing_attrs":
		return "missing attributes, dropped at verify"
	case "bad_timestamp":
		return "bad timestamp from app"
	case "oversize":
		return "oversize body from app"
	case "duplicate_id":
		return "duplicate id, counted at sink"
	case "connection_reset":
		return "connection reset from app"
	case "bad_signature":
		return "bad signature, rejected at verify"
	case "bad_in_batch":
		return "bad signature in batch, rejected at verify"
	case "stall":
		return "stall on sink"
	case "slow":
		return "slow on sink"
	default:
		return fault
	}
}

func actionLabel(action string) string {
	switch action {
	case "kill-main":
		return "kill main collector"
	case "stop-main":
		return "stop main collector"
	case "start-main":
		return "start main collector"
	case "kill-secondary":
		return "kill secondary collector"
	case "stop-secondary":
		return "stop secondary collector"
	case "start-secondary":
		return "start secondary collector"
	case "blackhole-main":
		return "blackhole on path to main"
	case "blackhole-secondary":
		return "blackhole on path to secondary"
	case "clear-faults":
		return "clear faults on all paths"
	case "low-bandwidth-main":
		return "low bandwidth on path to main"
	case "low-bandwidth-secondary":
		return "low bandwidth on path to secondary"
	case "low-bandwidth-sink":
		return "low bandwidth on path to sink"
	case "restart-sidecar":
		return "restart sidecar"
	default:
		return action
	}
}

func describe(injections []injection, steps []runStep) string {
	var b strings.Builder
	for _, inj := range injections {
		fmt.Fprintf(&b, "Every %d logs: %s. ", inj.Every, faultLabel(inj.Fault))
	}
	for _, step := range steps {
		fmt.Fprintf(&b, "After %d logs: %s. ", step.After, actionLabel(step.Action))
	}
	return b.String()
}

func (s *Server) playSteps(steps []runStep, runID string) {
	i := 0
	for i < len(steps) {
		after := steps[i].After
		deadline := time.Now().Add(2 * time.Minute)
		seen := false
		for time.Now().Before(deadline) {
			snap, err := s.spammerSnap()
			if err == nil && snap.RunID == runID && snap.Holding && snap.HoldAt == after {
				seen = true
				break
			}
			if err == nil && snap.RunID == runID && snap.Status == "finished" {
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
		if !seen {
			return
		}
		for i < len(steps) && steps[i].After == after {
			_ = s.doAction(steps[i].Action)
			i++
		}
		_ = s.spammerContinue()
	}
}

func (s *Server) waitSpammer(runID string) {
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		snap, err := s.spammerSnap()
		if err == nil && snap.RunID == runID && snap.Status == "finished" {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func (s *Server) spammerSnap() (spammerSnap, error) {
	resp, err := s.client().Get(s.SpammerURL + "/last-run")
	if err != nil {
		return spammerSnap{}, err
	}
	defer resp.Body.Close()
	var snap spammerSnap
	if err := json.NewDecoder(resp.Body).Decode(&snap); err != nil {
		return spammerSnap{}, err
	}
	return snap, nil
}

func (s *Server) spammerContinue() error {
	resp, err := s.client().Post(s.SpammerURL+"/continue", "application/json", bytes.NewReader(nil))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("continue: %s", resp.Status)
	}
	return nil
}

func (s *Server) doAction(action string) error {
	switch action {
	case "kill-main":
		return s.Docker.Kill(s.Main)
	case "stop-main":
		return s.Docker.Stop(s.Main)
	case "start-main":
		if err := s.Docker.Start(s.Main); err != nil {
			return err
		}
		s.waitURL(s.MainHealth)
		return nil
	case "kill-secondary":
		return s.Docker.Kill(s.Secondary)
	case "stop-secondary":
		return s.Docker.Stop(s.Secondary)
	case "start-secondary":
		if err := s.Docker.Start(s.Secondary); err != nil {
			return err
		}
		s.waitURL(s.SecondaryHealth)
		return nil
	case "blackhole-main":
		return s.toxic(http.MethodPost, "to-main", map[string]bool{"blackhole": true})
	case "blackhole-secondary":
		return s.toxic(http.MethodPost, "to-secondary", map[string]bool{"blackhole": true})
	case "clear-faults":
		return s.clearLinks()
	case "low-bandwidth-main":
		return s.toxic(http.MethodPost, "to-main", map[string]int{"bandwidth_bps": 256})
	case "low-bandwidth-secondary":
		return s.toxic(http.MethodPost, "to-secondary", map[string]int{"bandwidth_bps": 256})
	case "low-bandwidth-sink":
		return s.toxic(http.MethodPost, "to-sink", map[string]int{"bandwidth_bps": 256})
	case "restart-sidecar":
		return s.restartSidecar()
	default:
		return fmt.Errorf("unknown action %s", action)
	}
}

func (s *Server) restoreSteps(steps []runStep) {
	for i := len(steps) - 1; i >= 0; i-- {
		switch steps[i].Action {
		case "kill-main", "stop-main":
			_ = s.Docker.Start(s.Main)
			s.waitURL(s.MainHealth)
		case "kill-secondary", "stop-secondary":
			_ = s.Docker.Start(s.Secondary)
			s.waitURL(s.SecondaryHealth)
		case "blackhole-main", "low-bandwidth-main":
			_ = s.toxic(http.MethodDelete, "to-main", nil)
		case "blackhole-secondary", "low-bandwidth-secondary":
			_ = s.toxic(http.MethodDelete, "to-secondary", nil)
		case "low-bandwidth-sink":
			_ = s.toxic(http.MethodDelete, "to-sink", nil)
		case "restart-sidecar":
			running, err := s.Docker.Running(s.Sidecar)
			if err != nil || !running {
				_ = s.Docker.Start(s.Sidecar)
				s.waitURL(s.SidecarHealth)
			}
		}
	}
}

func (s *Server) clearLinks() error {
	var first error
	for _, link := range []string{"to-main", "to-secondary", "to-sink"} {
		if err := s.toxic(http.MethodDelete, link, nil); err != nil && first == nil {
			first = err
		}
	}
	return first
}

func (s *Server) restartSidecar() error {
	if err := s.Docker.Stop(s.Sidecar); err != nil {
		return err
	}
	if err := s.Docker.Start(s.Sidecar); err != nil {
		return err
	}
	s.waitURL(s.SidecarHealth)
	return nil
}

type scenario struct {
	Scenario
	injections []injection
	steps      []runStep
	batch      int
	prepare    func(*Server) error
	cleanup    func(*Server) error
	judge      func(sink.Stats) (bool, string)
}

func catalog() []scenario {
	none := func(*Server) error { return nil }
	list := []scenario{
		{Scenario: Scenarios()[0], prepare: none, cleanup: none, judge: judgeAllMain},
		{Scenario: Scenarios()[1], prepare: func(s *Server) error { return s.sinkDelay(400, false) }, cleanup: func(s *Server) error { return s.sinkDelay(0, false) }, judge: judgeSlow},
		{Scenario: Scenarios()[2], prepare: func(s *Server) error { return s.sinkDelay(6000, true) }, cleanup: func(s *Server) error { return s.sinkDelay(0, false) }, judge: judgeStall},
		{Scenario: Scenarios()[3], prepare: func(s *Server) error { return s.Docker.Stop(s.Main) }, cleanup: func(s *Server) error { return s.Docker.Start(s.Main) }, judge: judgeAllSecondary},
		{Scenario: Scenarios()[4], prepare: func(s *Server) error { return s.toxic(http.MethodPost, "to-main", map[string]bool{"blackhole": true}) }, cleanup: func(s *Server) error { return s.toxic(http.MethodDelete, "to-main", nil) }, judge: judgeAllSecondary},
		{Scenario: Scenarios()[5], prepare: func(s *Server) error { return s.Docker.Stop(s.Secondary) }, cleanup: func(s *Server) error { return s.Docker.Start(s.Secondary) }, judge: judgeAllMain},
		{Scenario: Scenarios()[6], prepare: func(s *Server) error {
			if err := s.Docker.Stop(s.Main); err != nil {
				return err
			}
			return s.Docker.Stop(s.Secondary)
		}, cleanup: func(s *Server) error {
			err1 := s.Docker.Start(s.Main)
			err2 := s.Docker.Start(s.Secondary)
			if err1 != nil {
				return err1
			}
			return err2
		}, judge: judgeArrived},
		{Scenario: Scenarios()[7], prepare: none, cleanup: none, judge: judgeMalformed},
		{Scenario: Scenarios()[8], prepare: func(s *Server) error {
			return s.toxic(http.MethodPost, "to-main", map[string]bool{"flip_signature": true})
		}, cleanup: func(s *Server) error { return s.toxic(http.MethodDelete, "to-main", nil) }, judge: judgeBadSignature},
		{Scenario: Scenarios()[9], prepare: func(s *Server) error { return s.Docker.Kill(s.Main) }, cleanup: func(s *Server) error { return s.Docker.Start(s.Main) }, judge: judgeAllSecondary},
		{Scenario: Scenarios()[10], prepare: none, cleanup: none, judge: judgeArrived},
	}
	return list
}

func stringsTrim(b []byte) string {
	if len(b) > 300 {
		b = b[:300]
	}
	return string(bytes.TrimSpace(b))
}
