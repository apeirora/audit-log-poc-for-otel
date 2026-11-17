package verifyprocessor

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/processor/processortest"

	"audit-loss/collector/components/hmac"
)

func TestVerifyDropsBadSignatureAndReports(t *testing.T) {
	key := []byte("dev-key")
	var reasons []string
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rejected" {
			t.Errorf("path %s", r.URL.Path)
		}
		var body struct {
			RunID    string `json:"run_id"`
			Sequence int    `json:"sequence"`
			Reason   string `json:"reason"`
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		reasons = append(reasons, body.Reason)
		w.WriteHeader(http.StatusOK)
	}))
	defer sink.Close()

	ld := plog.NewLogs()
	scope := ld.ResourceLogs().AppendEmpty().ScopeLogs().AppendEmpty()
	good := scope.LogRecords().AppendEmpty()
	good.Body().SetStr("audit log 1")
	good.Attributes().PutStr("demo.run_id", "run")
	good.Attributes().PutStr("demo.sequence", "1")
	good.Attributes().PutStr("demo.emit_time_unix_nano", "100")
	good.Attributes().PutStr("demo.scenario", "happy")
	good.Attributes().PutStr("audit.integrity.value", hmac.Sign(key, "run", "1", "100", "audit log 1"))
	bad := scope.LogRecords().AppendEmpty()
	bad.Body().SetStr("audit log 2")
	bad.Attributes().PutStr("demo.run_id", "run")
	bad.Attributes().PutStr("demo.sequence", "2")
	bad.Attributes().PutStr("demo.emit_time_unix_nano", "100")
	bad.Attributes().PutStr("demo.scenario", "happy")
	bad.Attributes().PutStr("audit.integrity.value", "00")
	missing := scope.LogRecords().AppendEmpty()
	missing.Body().SetStr("audit log 3")
	missing.Attributes().PutStr("demo.run_id", "run")
	missing.Attributes().PutStr("demo.sequence", "3")

	next := &capture{}
	proc := newVerify(t, sink.URL, next)
	if err := proc.ConsumeLogs(context.Background(), ld); err != nil {
		t.Fatal(err)
	}
	if next.count != 1 {
		t.Fatalf("forwarded %d", next.count)
	}
	if len(reasons) != 2 || reasons[0] != "bad_signature" || reasons[1] != "missing_attributes" {
		t.Fatalf("reasons %v", reasons)
	}
}

func TestVerifyEmptyBatchSkipsExport(t *testing.T) {
	ld := plog.NewLogs()
	rec := ld.ResourceLogs().AppendEmpty().ScopeLogs().AppendEmpty().LogRecords().AppendEmpty()
	rec.Body().SetStr("nope")
	rec.Attributes().PutStr("demo.run_id", "run")
	rec.Attributes().PutStr("demo.sequence", "9")
	rec.Attributes().PutStr("demo.emit_time_unix_nano", "100")
	rec.Attributes().PutStr("demo.scenario", "happy")
	rec.Attributes().PutStr("audit.integrity.value", "00")
	next := &capture{}
	proc := newVerify(t, "", next)
	if err := proc.ConsumeLogs(context.Background(), ld); err != nil {
		t.Fatal(err)
	}
	if next.called {
		t.Fatal("empty batch was forwarded")
	}
}

func newVerify(t *testing.T, sink string, next consumer.Logs) interface {
	ConsumeLogs(context.Context, plog.Logs) error
} {
	t.Helper()
	path := filepath.Join(t.TempDir(), "key.txt")
	if err := os.WriteFile(path, []byte("dev-key\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	proc, err := NewFactory().CreateLogs(
		context.Background(),
		processortest.NewNopSettings(component.MustNewType("verify")),
		&Config{KeyFile: path, SinkURL: sink},
		next,
	)
	if err != nil {
		t.Fatal(err)
	}
	return proc
}

type capture struct {
	called bool
	count  int
}

func (c *capture) ConsumeLogs(_ context.Context, ld plog.Logs) error {
	c.called = true
	c.count = ld.LogRecordCount()
	return nil
}

func (c *capture) Capabilities() consumer.Capabilities { return consumer.Capabilities{} }
