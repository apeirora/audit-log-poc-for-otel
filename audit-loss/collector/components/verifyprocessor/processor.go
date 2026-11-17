package verifyprocessor

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/processor/processorhelper"

	"audit-loss/collector/components/hmac"
)

type verifyProc struct {
	key    []byte
	sink   string
	client *http.Client
}

func (p *verifyProc) process(ctx context.Context, ld plog.Logs) (plog.Logs, error) {
	rls := ld.ResourceLogs()
	for i := 0; i < rls.Len(); i++ {
		sls := rls.At(i).ScopeLogs()
		for j := 0; j < sls.Len(); j++ {
			sls.At(j).LogRecords().RemoveIf(func(rec plog.LogRecord) bool {
				return p.drop(ctx, rec)
			})
		}
	}
	if ld.LogRecordCount() == 0 {
		return ld, processorhelper.ErrSkipProcessingData
	}
	return ld, nil
}

func (p *verifyProc) drop(ctx context.Context, rec plog.LogRecord) bool {
	runID := attr(rec, "demo.run_id")
	seq := attr(rec, "demo.sequence")
	emit := attr(rec, "demo.emit_time_unix_nano")
	body := rec.Body().AsString()
	if runID == "" || seq == "" || emit == "" || attr(rec, "demo.scenario") == "" {
		p.report(ctx, runID, seq, "missing_attributes")
		return true
	}
	if !hmac.Verify(p.key, runID, seq, emit, body, attr(rec, "audit.integrity.value")) {
		p.report(ctx, runID, seq, "bad_signature")
		return true
	}
	return false
}

func (p *verifyProc) report(ctx context.Context, runID, sequence, reason string) {
	if p.sink == "" || runID == "" {
		return
	}
	seq, _ := strconv.Atoi(sequence)
	raw, _ := json.Marshal(map[string]any{"run_id": runID, "sequence": seq, "reason": reason})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(p.sink, "/")+"/rejected", bytes.NewReader(raw))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.client.Do(req)
	if err != nil {
		return
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
}

func attr(rec plog.LogRecord, key string) string {
	v, ok := rec.Attributes().Get(key)
	if !ok {
		return ""
	}
	return v.AsString()
}
