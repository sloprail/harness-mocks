package replay

import (
	"encoding/json"
	"strings"
)

// parseHookLog is a hook log as objects, one per line. A hook's own line that is not JSON (a hook that
// printed text of its own, or JSON the shell mangled) is kept as {"raw": line}, in its place: the log is
// what the hooks wrote, not only what the harness sent them. Consecutive identical raw lines are one
// (a background job that ticks while the run lasts writes as many as the run was long, which the mock,
// with no model to wait for, does not reproduce: that it ticks, and stops with the run, is the proof).
func parseHookLog(text string) ([]map[string]any, error) {
	var out []map[string]any
	last := ""
	for _, l := range strings.Split(text, "\n") {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		var m map[string]any
		if json.Unmarshal([]byte(l), &m) != nil || m == nil {
			if l == last {
				continue
			}
			last = l
			m = map[string]any{"raw": l}
		} else {
			last = ""
		}
		out = append(out, m)
	}
	return out, nil
}
