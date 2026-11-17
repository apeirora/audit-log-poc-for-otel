package toxic

import (
	"bytes"
	"encoding/json"
	"io"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"
)

type Fault struct {
	LatencyMS     int  `json:"latency_ms"`
	JitterMS      int  `json:"jitter_ms"`
	Stall         bool `json:"stall"`
	Reset         bool `json:"reset"`
	Blackhole     bool `json:"blackhole"`
	DropPercent   int  `json:"drop_percent"`
	Status        int  `json:"status"`
	BandwidthBPS  int  `json:"bandwidth_bps"`
	Truncate      bool `json:"truncate"`
	FlipSignature bool `json:"flip_signature"`
	MaxBytes      int  `json:"max_bytes"`
}

type Server struct {
	Upstreams map[string]string
	Client    *http.Client
	mu        sync.Mutex
	faults    map[string]Fault
	roll      func(n int) int
}

func New(upstreams map[string]string) *Server {
	return &Server{
		Upstreams: upstreams,
		Client:    &http.Client{Timeout: 30 * time.Second},
		faults:    map[string]Fault{},
		roll:      rand.Intn,
	}
}

func (s *Server) Control() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /faults", func(w http.ResponseWriter, _ *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		writeJSON(w, http.StatusOK, s.faults)
	})
	mux.HandleFunc("POST /faults/{link}", s.setFault)
	mux.HandleFunc("DELETE /faults/{link}", s.clearFault)
	return mux
}

func (s *Server) setFault(w http.ResponseWriter, r *http.Request) {
	link := r.PathValue("link")
	if _, ok := s.Upstreams[link]; !ok {
		http.Error(w, "unknown link", http.StatusNotFound)
		return
	}
	var fault Fault
	if err := json.NewDecoder(r.Body).Decode(&fault); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	s.faults[link] = fault
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, fault)
}

func (s *Server) clearFault(w http.ResponseWriter, r *http.Request) {
	link := r.PathValue("link")
	s.mu.Lock()
	delete(s.faults, link)
	s.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) Proxy(link string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 8<<20))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		fault := s.fault(link)
		if fault.DropPercent > 0 && s.roll(100) < fault.DropPercent {
			fault.Blackhole = true
		}
		if fault.MaxBytes > 0 && len(body) > fault.MaxBytes {
			http.Error(w, "too large", http.StatusRequestEntityTooLarge)
			return
		}
		if fault.Status != 0 {
			w.WriteHeader(fault.Status)
			return
		}
		if fault.Reset {
			hj, ok := w.(http.Hijacker)
			if !ok {
				http.Error(w, "reset unsupported", http.StatusBadGateway)
				return
			}
			conn, _, err := hj.Hijack()
			if err != nil {
				return
			}
			_ = conn.Close()
			return
		}
		if fault.Stall || fault.Blackhole {
			<-r.Context().Done()
			return
		}
		wait := time.Duration(fault.LatencyMS) * time.Millisecond
		if fault.JitterMS > 0 {
			wait += time.Duration(s.roll(fault.JitterMS+1)) * time.Millisecond
		}
		if wait > 0 {
			timer := time.NewTimer(wait)
			select {
			case <-r.Context().Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
		if fault.FlipSignature {
			body = FlipSignature(body)
		}
		if fault.Truncate && len(body) > 1 {
			body = body[:len(body)/2]
		}
		upstream := strings.TrimRight(s.Upstreams[link], "/") + r.URL.RequestURI()
		var payload io.Reader = bytes.NewReader(body)
		if fault.BandwidthBPS > 0 {
			payload = slowReader{r: bytes.NewReader(body), bps: fault.BandwidthBPS}
		}
		req, err := http.NewRequestWithContext(r.Context(), r.Method, upstream, payload)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		req.ContentLength = int64(len(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := s.Client.Do(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		for k, vals := range resp.Header {
			if hop(k) {
				continue
			}
			for _, v := range vals {
				w.Header().Add(k, v)
			}
		}
		w.WriteHeader(resp.StatusCode)
		if fault.BandwidthBPS > 0 {
			copySlow(w, resp.Body, fault.BandwidthBPS)
			return
		}
		_, _ = io.Copy(w, resp.Body)
	})
}

func (s *Server) fault(link string) Fault {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.faults[link]
}

func FlipSignature(body []byte) []byte {
	idx := bytes.Index(body, []byte("audit.integrity.value"))
	if idx < 0 {
		return append([]byte(nil), body...)
	}
	windowEnd := idx + 240
	if windowEnd > len(body) {
		windowEnd = len(body)
	}
	rel := bytes.Index(body[idx:windowEnd], []byte("stringValue"))
	if rel < 0 {
		return append([]byte(nil), body...)
	}
	pos := idx + rel + len("stringValue")
	for pos < len(body) && (body[pos] == '"' || body[pos] == ':' || body[pos] == ' ') {
		pos++
	}
	if pos >= len(body) {
		return append([]byte(nil), body...)
	}
	out := append([]byte(nil), body...)
	out[pos] = flipHex(out[pos])
	return out
}

func flipHex(b byte) byte {
	if b == '0' {
		return '1'
	}
	return '0'
}

type slowReader struct {
	r   io.Reader
	bps int
}

func (s slowReader) Read(p []byte) (int, error) {
	n, err := s.r.Read(p)
	if n > 0 && s.bps > 0 {
		time.Sleep(time.Duration(float64(n) / float64(s.bps) * float64(time.Second)))
	}
	return n, err
}

func copySlow(dst io.Writer, src io.Reader, bps int) {
	buf := make([]byte, 1024)
	for {
		n, err := src.Read(buf)
		if n > 0 {
			_, _ = dst.Write(buf[:n])
			time.Sleep(time.Duration(float64(n) / float64(bps) * float64(time.Second)))
		}
		if err != nil {
			return
		}
	}
}

func hop(name string) bool {
	switch strings.ToLower(name) {
	case "connection", "keep-alive", "proxy-connection", "transfer-encoding", "upgrade", "content-length":
		return true
	default:
		return false
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}
