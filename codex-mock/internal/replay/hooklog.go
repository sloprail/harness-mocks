package replay

import (
	"encoding/json"
	"strings"
)

// parseHookLog is a log as objects, one per line. A line that is not JSON (a hook that printed text of
// its own, JSON the shell mangled, or a text-mode stdout line) is kept as {"raw": line}, in its place.
// Nothing is collapsed: a line written twice is two.
func parseHookLog(text string) ([]map[string]any, error) {
	var out []map[string]any
	for _, l := range strings.Split(text, "\n") {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		var m map[string]any
		if json.Unmarshal([]byte(l), &m) != nil || m == nil {
			m = map[string]any{"raw": l}
		}
		out = append(out, m)
	}
	return out, nil
}

// tickLine is the one hook line a background job writes as many times as the run lasted. The mock, with no
// model to wait for, does not reproduce the count: that it ticks at all, and stops with the run, is the proof.
const tickLine = "{hook_event_name:BackgroundTick}"

// parseHooks is a hook log (not a stdout) as parseHookLog reads it, a run of consecutive tick lines being
// one masked line that keeps the key and drops the count.
func parseHooks(text string) ([]map[string]any, error) {
	log, err := parseHookLog(text)
	var out []map[string]any
	for _, m := range log {
		if m["raw"] == tickLine {
			m = map[string]any{"raw": tickLine + " x<N>"}
			if n := len(out); n > 0 && out[n-1]["raw"] == m["raw"] {
				continue
			}
		}
		out = append(out, m)
	}
	return out, err
}
