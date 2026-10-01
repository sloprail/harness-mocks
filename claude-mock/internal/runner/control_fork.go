package runner

import "github.com/sloprail/harness-mocks/internal/session"

// claudeForkSchema is how Claude Code's records show a fork what to carry: a
// compact_boundary system record, the isCompactSummary user record after it,
// and the uuids the boundary's compactMetadata lists as preserved.
var claudeForkSchema = session.ForkSchema{
	UUID: "uuid", Parent: "parentUuid", SessionKey: "sessionId",
	IsBoundary: func(rec session.Record) bool {
		return rec["type"] == "system" && rec["subtype"] == "compact_boundary"
	},
	IsSummary: func(rec session.Record) bool {
		v, _ := rec["isCompactSummary"].(bool)
		return v
	},
	Preserved: preservedUUIDs,
}

// forkSegment is the records of a transcript a fork carries, under session id
// newID (see forkTranscript).
func forkSegment(data []byte, newID string) []map[string]any {
	return session.Fork(session.ParseRecords(data, "uuid"), newID, claudeForkSchema)
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
