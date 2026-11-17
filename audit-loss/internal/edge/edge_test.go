package edge

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"audit-loss/internal/integrity"
	"audit-loss/internal/otlpjson"
	"audit-loss/internal/toxic"
)

func TestSignThenVerifyAndFlip(t *testing.T) {
	key := []byte("dev-key")
	var stored []byte
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stored, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()
	var rejected int
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rejected++
		w.WriteHeader(http.StatusOK)
	}))
	defer sink.Close()

	raw, err := otlpjson.Build([]otlpjson.Record{{
		Sequence: "1",
		Scenario: "happy",
		RunID:    "run",
		Emit:     "100",
		Body:     "audit log 1",
	}})
	if err != nil {
		t.Fatal(err)
	}
	signRR := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/logs", bytesReader(raw))
	SignHandler(key, upstream.URL, http.DefaultClient).ServeHTTP(signRR, req)
	if signRR.Code != http.StatusOK {
		t.Fatalf("sign %d %s", signRR.Code, signRR.Body.String())
	}
	doc, err := otlpjson.Parse(stored)
	if err != nil {
		t.Fatal(err)
	}
	otlpjson.Walk(doc, func(rec map[string]any) {
		if !integrity.Verify(key, "run", "1", "100", "audit log 1", otlpjson.GetAttr(rec, "audit.integrity.value")) {
			t.Fatal("signed record did not verify")
		}
	})

	verifyUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("bad signature was forwarded")
	}))
	defer verifyUpstream.Close()
	flipped := toxic.FlipSignature(stored)
	rr := httptest.NewRecorder()
	VerifyHandler(key, verifyUpstream.URL, sink.URL, http.DefaultClient).ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/v1/logs", bytesReader(flipped)))
	if rr.Code != http.StatusOK {
		t.Fatalf("verify %d", rr.Code)
	}
	if rejected != 1 {
		t.Fatalf("rejected %d", rejected)
	}
}

func TestInjectedBadSignatureDoesNotVerify(t *testing.T) {
	key := []byte("dev-key")
	raw, err := otlpjson.Build([]otlpjson.Record{{
		Sequence: "4",
		Scenario: "custom",
		RunID:    "run",
		Emit:     "100",
		Body:     "audit log 4",
		Attrs:    map[string]string{"demo.inject": "bad_signature"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	var stored []byte
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stored, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()
	rr := httptest.NewRecorder()
	SignHandler(key, upstream.URL, http.DefaultClient).ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/v1/logs", bytesReader(raw)))
	if rr.Code != http.StatusOK {
		t.Fatal(rr.Body.String())
	}
	doc, err := otlpjson.Parse(stored)
	if err != nil {
		t.Fatal(err)
	}
	otlpjson.Walk(doc, func(rec map[string]any) {
		if integrity.Verify(key, "run", "4", "100", "audit log 4", otlpjson.GetAttr(rec, "audit.integrity.value")) {
			t.Fatal("injected bad signature still verified")
		}
	})
}

func TestMissingAttributesDroppedBeforeCollector(t *testing.T) {
	key := []byte("dev-key")
	raw, err := otlpjson.Build([]otlpjson.Record{
		{Sequence: "1", Scenario: "custom", RunID: "run", Emit: "100", Body: "audit log 1"},
		{Sequence: "", Scenario: "custom", RunID: "run", Emit: "100", Body: "audit log 2"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var signed []byte
	signUp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		signed, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer signUp.Close()
	signRR := httptest.NewRecorder()
	SignHandler(key, signUp.URL, http.DefaultClient).ServeHTTP(signRR, httptest.NewRequest(http.MethodPost, "/v1/logs", bytesReader(raw)))
	if signRR.Code != http.StatusOK {
		t.Fatal(signRR.Body.String())
	}

	var forwarded int
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		doc, err := otlpjson.Parse(body)
		if err != nil {
			t.Errorf("parse %v", err)
			http.Error(w, "bad", http.StatusBadRequest)
			return
		}
		otlpjson.Walk(doc, func(rec map[string]any) {
			forwarded++
			if otlpjson.GetAttr(rec, "demo.sequence") == "" {
				t.Error("record with missing attributes was forwarded")
			}
		})
		w.WriteHeader(http.StatusOK)
	}))
	defer collector.Close()
	var rejected int
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rejected++
		w.WriteHeader(http.StatusOK)
	}))
	defer sink.Close()
	rr := httptest.NewRecorder()
	VerifyHandler(key, collector.URL, sink.URL, http.DefaultClient).ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/v1/logs", bytesReader(signed)))
	if rr.Code != http.StatusOK {
		t.Fatalf("verify %d", rr.Code)
	}
	if forwarded != 1 || rejected != 1 {
		t.Fatalf("forwarded %d rejected %d", forwarded, rejected)
	}
}

func TestMalformedIsRejectedBeforeSign(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("malformed body was forwarded")
	}))
	defer upstream.Close()
	rr := httptest.NewRecorder()
	SignHandler([]byte("k"), upstream.URL, http.DefaultClient).ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/v1/logs", bytesReader([]byte(`{"not":`))))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status %d", rr.Code)
	}
}

func bytesReader(b []byte) *readCloser {
	return &readCloser{b: b}
}

type readCloser struct {
	b []byte
	n int
}

func (r *readCloser) Read(p []byte) (int, error) {
	if r.n >= len(r.b) {
		return 0, io.EOF
	}
	n := copy(p, r.b[r.n:])
	r.n += n
	return n, nil
}

func (r *readCloser) Close() error { return nil }
