package signprocessor

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/processor/processortest"

	"audit-loss/collector/components/hmac"
)

func TestSignAddsHMACAndCanFlipIt(t *testing.T) {
	keyPath := writeKey(t, "dev-key")
	good := record("run", "1", "100", "happy", "audit log 1", "")
	bad := record("run", "4", "100", "custom", "audit log 4", "bad_signature")
	ld := plog.NewLogs()
	scope := ld.ResourceLogs().AppendEmpty().ScopeLogs().AppendEmpty()
	good.MoveTo(scope.LogRecords().AppendEmpty())
	bad.MoveTo(scope.LogRecords().AppendEmpty())

	next := &capture{}
	proc := newSign(t, keyPath, next)
	if err := proc.ConsumeLogs(context.Background(), ld); err != nil {
		t.Fatal(err)
	}
	if next.ld.LogRecordCount() != 2 {
		t.Fatalf("records %d", next.ld.LogRecordCount())
	}
	recs := next.ld.ResourceLogs().At(0).ScopeLogs().At(0).LogRecords()
	if !hmac.Verify([]byte("dev-key"), "run", "1", "100", "audit log 1", attr(recs.At(0), "audit.integrity.value")) {
		t.Fatal("signed record did not verify")
	}
	if hmac.Verify([]byte("dev-key"), "run", "4", "100", "audit log 4", attr(recs.At(1), "audit.integrity.value")) {
		t.Fatal("injected bad signature still verified")
	}
}

func newSign(t *testing.T, keyPath string, next consumer.Logs) processorLogs {
	t.Helper()
	proc, err := NewFactory().CreateLogs(
		context.Background(),
		processortest.NewNopSettings(component.MustNewType("sign")),
		&Config{KeyFile: keyPath},
		next,
	)
	if err != nil {
		t.Fatal(err)
	}
	return proc
}

type processorLogs interface {
	ConsumeLogs(context.Context, plog.Logs) error
}

type capture struct {
	ld plog.Logs
}

func (c *capture) ConsumeLogs(_ context.Context, ld plog.Logs) error {
	c.ld = ld
	return nil
}

func (c *capture) Capabilities() consumer.Capabilities { return consumer.Capabilities{} }

func record(runID, seq, emit, scenario, body, inject string) plog.LogRecord {
	ld := plog.NewLogs()
	rec := ld.ResourceLogs().AppendEmpty().ScopeLogs().AppendEmpty().LogRecords().AppendEmpty()
	rec.Body().SetStr(body)
	rec.Attributes().PutStr("demo.run_id", runID)
	rec.Attributes().PutStr("demo.sequence", seq)
	rec.Attributes().PutStr("demo.emit_time_unix_nano", emit)
	rec.Attributes().PutStr("demo.scenario", scenario)
	if inject != "" {
		rec.Attributes().PutStr("demo.inject", inject)
	}
	return rec
}

func writeKey(t *testing.T, key string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "key.txt")
	if err := os.WriteFile(path, []byte(key+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
