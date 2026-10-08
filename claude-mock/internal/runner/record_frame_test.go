package runner

import (
	"encoding/json"
	"regexp"
	"testing"
)

// A stream frame carries when it was written, as every recorded assistant and
// user frame does; a scenario's own timestamp is kept.
func TestStampFrameAddsATimestamp(t *testing.T) {
	var m map[string]any
	if err := json.Unmarshal(stampFrame(Config{SessionID: "s"}, []byte(`{"type":"assistant","message":{"content":[]}}`)), &m); err != nil {
		t.Fatal(err)
	}
	if ts, _ := m["timestamp"].(string); !regexp.MustCompile(`^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d\.\d{3}Z$`).MatchString(ts) {
		t.Fatalf("timestamp %v", m["timestamp"])
	}
	if err := json.Unmarshal(stampFrame(Config{}, []byte(`{"type":"user","timestamp":"2026-01-01T00:00:00.000Z"}`)), &m); err != nil || m["timestamp"] != "2026-01-01T00:00:00.000Z" {
		t.Fatalf("the scenario's own timestamp is replaced: %v %v", m["timestamp"], err)
	}
}
