package otlpjson

import "testing"

func TestBuildParseAndSignAttr(t *testing.T) {
	raw, err := Build([]Record{{
		Sequence: "1",
		Scenario: "happy",
		RunID:    "run",
		Emit:     "100",
		Body:     "audit log 1",
	}})
	if err != nil {
		t.Fatal(err)
	}
	doc, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	var got string
	Walk(doc, func(rec map[string]any) {
		got = GetAttr(rec, "demo.sequence")
		SetAttr(rec, "audit.integrity.value", "abc")
	})
	if got != "1" {
		t.Fatalf("sequence %q", got)
	}
	out, err := Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	again, err := Parse(out)
	if err != nil {
		t.Fatal(err)
	}
	Walk(again, func(rec map[string]any) {
		if GetAttr(rec, "audit.integrity.value") != "abc" {
			t.Fatal("signature attribute was not kept")
		}
	})
	n := Filter(again, func(rec map[string]any) bool {
		return GetAttr(rec, "demo.sequence") == "2"
	})
	if n != 0 {
		t.Fatalf("kept %d", n)
	}
}
