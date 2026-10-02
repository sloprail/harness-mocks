package e2e

import (
	"encoding/json"
	"strings"
	"testing"
)

// toolOutputs is the text of every tool-result record in a rollout, whatever
// its wire form: the real harness writes a list of input_text parts
// (runs/posttool-block: "Script failed ... Script error:\n<reason>"), the mock
// a plain string. Only what the agent was told is compared, not the form.
func toolOutputs(t *testing.T, rollout string) []string {
	t.Helper()
	var outs []string
	for _, line := range strings.Split(rollout, "\n") {
		var rec struct {
			Payload struct {
				Type   string          `json:"type"`
				Output json.RawMessage `json:"output"`
			} `json:"payload"`
		}
		if json.Unmarshal([]byte(line), &rec) != nil || !strings.HasSuffix(rec.Payload.Type, "_call_output") {
			continue
		}
		var s string
		if json.Unmarshal(rec.Payload.Output, &s) == nil {
			outs = append(outs, s)
			continue
		}
		var parts []struct {
			Text string `json:"text"`
		}
		if json.Unmarshal(rec.Payload.Output, &parts) == nil {
			var b strings.Builder
			for _, p := range parts {
				b.WriteString(p.Text)
			}
			outs = append(outs, b.String())
		}
	}
	return outs
}

// resultTold is whether the agent was told, as the result of some tool call, a
// text containing want and none containing unwanted.
func resultTold(t *testing.T, rollout, want, unwanted string) bool {
	t.Helper()
	found := false
	for _, o := range toolOutputs(t, rollout) {
		if strings.Contains(o, unwanted) {
			return false
		}
		found = found || strings.Contains(o, want)
	}
	return found
}

// developerTexts is the text of every role=developer message in a rollout, in
// order: what hooks added as context.
func developerTexts(t *testing.T, rollout string) []string {
	t.Helper()
	var out []string
	for _, line := range strings.Split(rollout, "\n") {
		var rec struct {
			Type    string `json:"type"`
			Payload struct {
				Type    string `json:"type"`
				Role    string `json:"role"`
				Content []struct {
					Text string `json:"text"`
				} `json:"content"`
			} `json:"payload"`
		}
		if json.Unmarshal([]byte(line), &rec) != nil || rec.Type != "response_item" ||
			rec.Payload.Type != "message" || rec.Payload.Role != "developer" {
			continue
		}
		var b strings.Builder
		for _, c := range rec.Payload.Content {
			b.WriteString(c.Text)
		}
		out = append(out, b.String())
	}
	return out
}
