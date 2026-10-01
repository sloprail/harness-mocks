package scenario

import (
	"strings"
	"testing"
)

func TestRecordType(t *testing.T) {
	known := map[string]bool{"assistant": true, "result": true}
	if typ, err := RecordType([]byte(`{"type":"Result"}`), known); err != nil || typ != "Result" {
		t.Fatalf("RecordType = %q, %v", typ, err)
	}
	for line, want := range map[string]string{
		`not json`:        "not valid JSON",
		`{"a":1}`:         "missing required field",
		`{"type":"nope"}`: "unknown record type",
	} {
		if _, err := RecordType([]byte(line), known); err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("RecordType(%s) err = %v, want %q", line, err, want)
		}
	}
}

func TestResult_OnlyTheLastHeldResultIsWrittenOnce(t *testing.T) {
	var r Result
	var out []string
	write := func(l []byte) { out = append(out, string(l)) }
	if r.Finish(write) {
		t.Fatal("finished without a result")
	}
	r.Hold([]byte("one"))
	r.Continue() // the turn went on: that result is dropped
	r.Hold([]byte("two"))
	r.Hold([]byte("three"))
	if !r.Finish(write) || r.Finish(write) {
		t.Fatal("the result must be written exactly once")
	}
	if len(out) != 1 || out[0] != "three" {
		t.Fatalf("written = %v", out)
	}
}
