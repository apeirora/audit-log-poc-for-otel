package otlpjson

import (
	"bytes"
	"encoding/json"
)

type Record struct {
	Sequence string
	Scenario string
	RunID    string
	Emit     string
	Body     string
	Attrs    map[string]string
}

func Build(records []Record) ([]byte, error) {
	logs := make([]any, 0, len(records))
	for _, rec := range records {
		attrs := []any{
			attr("demo.sequence", rec.Sequence),
			attr("demo.scenario", rec.Scenario),
			attr("demo.run_id", rec.RunID),
			attr("demo.emit_time_unix_nano", rec.Emit),
		}
		for k, v := range rec.Attrs {
			attrs = append(attrs, attr(k, v))
		}
		logs = append(logs, map[string]any{
			"timeUnixNano": rec.Emit,
			"severityText": "INFO",
			"body":         map[string]any{"stringValue": rec.Body},
			"attributes":   attrs,
		})
	}
	doc := map[string]any{
		"resourceLogs": []any{
			map[string]any{
				"resource": map[string]any{
					"attributes": []any{attr("service.name", "app")},
				},
				"scopeLogs": []any{
					map[string]any{"logRecords": logs},
				},
			},
		},
	}
	return Marshal(doc)
}

func Marshal(doc map[string]any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(doc); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func Parse(body []byte) (map[string]any, error) {
	var doc map[string]any
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	if err := dec.Decode(&doc); err != nil {
		return nil, err
	}
	return doc, nil
}

func Walk(doc map[string]any, fn func(rec map[string]any)) {
	for _, rl := range asSlice(doc["resourceLogs"]) {
		rlm, _ := rl.(map[string]any)
		for _, sl := range asSlice(rlm["scopeLogs"]) {
			slm, _ := sl.(map[string]any)
			for _, rec := range asSlice(slm["logRecords"]) {
				if m, ok := rec.(map[string]any); ok {
					fn(m)
				}
			}
		}
	}
}

func Filter(doc map[string]any, keep func(rec map[string]any) bool) int {
	kept := 0
	for _, rl := range asSlice(doc["resourceLogs"]) {
		rlm, _ := rl.(map[string]any)
		for _, sl := range asSlice(rlm["scopeLogs"]) {
			slm, _ := sl.(map[string]any)
			next := make([]any, 0)
			for _, rec := range asSlice(slm["logRecords"]) {
				m, ok := rec.(map[string]any)
				if !ok {
					continue
				}
				if keep(m) {
					next = append(next, m)
					kept++
				}
			}
			slm["logRecords"] = next
		}
	}
	return kept
}

func GetAttr(rec map[string]any, key string) string {
	for _, item := range asSlice(rec["attributes"]) {
		am, _ := item.(map[string]any)
		if am["key"] != key {
			continue
		}
		val, _ := am["value"].(map[string]any)
		if s, ok := val["stringValue"].(string); ok {
			return s
		}
		switch n := val["intValue"].(type) {
		case string:
			return n
		case json.Number:
			return n.String()
		}
	}
	return ""
}

func SetAttr(rec map[string]any, key, value string) {
	attrs := asSlice(rec["attributes"])
	for _, item := range attrs {
		am, _ := item.(map[string]any)
		if am["key"] == key {
			am["value"] = map[string]any{"stringValue": value}
			return
		}
	}
	rec["attributes"] = append(attrs, attr(key, value))
}

func Body(rec map[string]any) string {
	body, _ := rec["body"].(map[string]any)
	s, _ := body["stringValue"].(string)
	return s
}

func attr(key, value string) map[string]any {
	return map[string]any{
		"key":   key,
		"value": map[string]any{"stringValue": value},
	}
}

func asSlice(v any) []any {
	s, _ := v.([]any)
	return s
}
