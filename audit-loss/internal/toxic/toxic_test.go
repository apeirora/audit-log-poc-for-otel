package toxic

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestFlipSignatureChangesOneHexDigit(t *testing.T) {
	in := []byte(`{"key":"audit.integrity.value","value":{"stringValue":"abcdef"}}`)
	out := FlipSignature(in)
	if bytes.Equal(in, out) {
		t.Fatal("signature was not flipped")
	}
	if !bytes.Contains(out, []byte("0bcdef")) {
		t.Fatalf("body %s", out)
	}
}

func TestStatusShortCircuitsAndBlackholeDoesNotForward(t *testing.T) {
	var hits atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("up"))
	}))
	defer upstream.Close()
	srv := New(map[string]string{"to-main": upstream.URL})

	set := httptest.NewRequest(http.MethodPost, "/faults/to-main", bytes.NewBufferString(`{"status":503}`))
	rr := httptest.NewRecorder()
	srv.Control().ServeHTTP(rr, set)
	if rr.Code != http.StatusOK {
		t.Fatal(rr.Body.String())
	}
	proxyRR := httptest.NewRecorder()
	srv.Proxy("to-main").ServeHTTP(proxyRR, httptest.NewRequest(http.MethodPost, "/v1/logs", bytes.NewBufferString(`{"ok":true}`)))
	if proxyRR.Code != http.StatusServiceUnavailable || hits.Load() != 0 {
		t.Fatalf("status %d hits %d", proxyRR.Code, hits.Load())
	}

	clear := httptest.NewRequest(http.MethodDelete, "/faults/to-main", nil)
	srv.Control().ServeHTTP(httptest.NewRecorder(), clear)
	proxyRR = httptest.NewRecorder()
	srv.Proxy("to-main").ServeHTTP(proxyRR, httptest.NewRequest(http.MethodPost, "/v1/logs", bytes.NewBufferString(`{"ok":true}`)))
	if proxyRR.Code != http.StatusOK || string(proxyRR.Body.Bytes()) != "up" {
		t.Fatalf("cleared status %d %s", proxyRR.Code, proxyRR.Body.String())
	}

	set = httptest.NewRequest(http.MethodPost, "/faults/to-main", bytes.NewBufferString(`{"blackhole":true}`))
	srv.Control().ServeHTTP(httptest.NewRecorder(), set)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest(http.MethodPost, "/v1/logs", bytes.NewBufferString(`{}`)).WithContext(ctx)
	before := hits.Load()
	proxyRR = httptest.NewRecorder()
	srv.Proxy("to-main").ServeHTTP(proxyRR, req)
	if proxyRR.Body.Len() != 0 || hits.Load() != before {
		t.Fatalf("blackhole forwarded hits %d body %q", hits.Load(), proxyRR.Body.String())
	}
}

func TestLowBandwidthStillDeliversTheBody(t *testing.T) {
	const payload = `{"audit":"log"}`
	var got string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got = string(body)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready-ok"))
	}))
	defer upstream.Close()
	srv := New(map[string]string{"to-sink": upstream.URL})
	set := httptest.NewRequest(http.MethodPost, "/faults/to-sink", bytes.NewBufferString(`{"bandwidth_bps":1000000}`))
	srv.Control().ServeHTTP(httptest.NewRecorder(), set)
	rr := httptest.NewRecorder()
	srv.Proxy("to-sink").ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/v1/logs", bytes.NewBufferString(payload)))
	if rr.Code != http.StatusOK || got != payload || rr.Body.String() != "ready-ok" {
		t.Fatalf("status %d got %q resp %q", rr.Code, got, rr.Body.String())
	}
}
