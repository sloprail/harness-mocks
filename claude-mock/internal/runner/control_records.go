package runner

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
