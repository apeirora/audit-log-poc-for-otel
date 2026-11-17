package signprocessor

import (
	"context"

	"go.opentelemetry.io/collector/pdata/plog"

	"audit-loss/collector/components/hmac"
)

type signProc struct {
	key []byte
}

func (p *signProc) process(_ context.Context, ld plog.Logs) (plog.Logs, error) {
	rls := ld.ResourceLogs()
	for i := 0; i < rls.Len(); i++ {
		sls := rls.At(i).ScopeLogs()
		for j := 0; j < sls.Len(); j++ {
			recs := sls.At(j).LogRecords()
			for k := 0; k < recs.Len(); k++ {
				signRecord(p.key, recs.At(k))
			}
		}
	}
	return ld, nil
}

func signRecord(key []byte, rec plog.LogRecord) {
	sig := hmac.Sign(key, attr(rec, "demo.run_id"), attr(rec, "demo.sequence"), attr(rec, "demo.emit_time_unix_nano"), rec.Body().AsString())
	if attr(rec, "demo.inject") == "bad_signature" && len(sig) > 0 {
		sig = flipHex(sig)
	}
	rec.Attributes().PutStr("audit.integrity.value", sig)
}

func attr(rec plog.LogRecord, key string) string {
	v, ok := rec.Attributes().Get(key)
	if !ok {
		return ""
	}
	return v.AsString()
}

func flipHex(sig string) string {
	b := []byte(sig)
	if b[len(b)-1] == '0' {
		b[len(b)-1] = '1'
	} else {
		b[len(b)-1] = '0'
	}
	return string(b)
}
