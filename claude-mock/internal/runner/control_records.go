package runner

import "errors"

import "github.com/sloprail/harness-mocks/internal/session"

// isCompactBoundary: a compact_boundary system record, where Claude Code's
// compaction starts.
func isCompactBoundary(rec session.Record) bool {
	return rec["type"] == "system" && rec["subtype"] == "compact_boundary"
}

// isCompactSummary: the isCompactSummary user record after a boundary.
func isCompactSummary(rec session.Record) bool {
	v, _ := rec["isCompactSummary"].(bool)
	return v
}

// preservedUUIDs is the list of records a compact_boundary says the
// compaction kept, in order.
func preservedUUIDs(boundary map[string]any) []string {
	meta, _ := boundary["compactMetadata"].(map[string]any)
	pm, _ := meta["preservedMessages"].(map[string]any)
	raw, _ := pm["uuids"].([]any)
	var out []string
	for _, v := range raw {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// errAPIRetry refuses a scenario that streams the system/api_retry event the real run emits when a
// model API request fails and is retried: the mock has no model API to fail, and a retry cannot be
// brought about on demand, so none is recorded (adr/fail-fast-unimplemented).
// sr:docs https://code.claude.com/docs/en/headless#handle-api-retries
var errAPIRetry = errors.New("claude-mock: the system/api_retry event is not implemented by the mock (it has no model API to retry): it is refused rather than ignored")

// refusedTools are the tools the mock does not implement and refuses by name when a scenario calls
// them: Monitor (watches a background process) and Workflow (runs a background workflow), which
// keep a `claude -p` run open (adr/fail-fast-unimplemented).
// sr:docs https://code.claude.com/docs/en/headless#background-tasks-at-exit
var refusedTools = map[string]bool{"Monitor": true, "Workflow": true}
