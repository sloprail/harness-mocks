package runner

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// forkTranscript writes dest as a FORK of the session fromID under the new
// session id newID — what `--resume <id> --fork-session` leaves.
//
// Measured on claude 2.1.282 (a controlled fork) and on the real forks on one
// machine (EVIDENCE.md):
//
//   - A session never compacted forks whole: every record, origin included,
//     its parentUuid unchanged, with sessionId rewritten to the fork's — as
//     every 2.1.280+ fork on the machine did.
//   - A compacted session forks from its LAST compact_boundary: a verbatim copy
//     of the boundary (same uuid and logicalParentUuid; sessionId rewritten),
//     then what follows it up to and including the summary, then the records
//     the boundary lists as preserved — re-parented into one chain after the
//     summary — then everything after the summary, the first of it re-parented
//     onto the last preserved record. That is the order of all 19 real
//     transcripts that open on a compact_boundary.
//
// The source file is only read.
func forkTranscript(configDir, cwd, fromID, dest, newID string) error {
	if fileExists(dest) {
		return fmt.Errorf("fork: %s already exists", dest)
	}
	src := sessionFilePathIfExists(configDir, cwd, fromID)
	if src == "" {
		return &ErrNoConversation{SessionID: fromID}
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("fork: %w", err)
	}
	segment := forkSegment(data)

	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	var buf []byte
	for _, line := range mockPreambleRecords(newID) {
		buf = append(append(buf, line...), '\n')
	}
	for _, rec := range segment {
		rec["sessionId"] = newID
		b, err := marshalRecord(rec)
		if err != nil {
			continue
		}
		buf = append(append(buf, b...), '\n')
	}
	return os.WriteFile(dest, buf, 0o644)
}

// forkSegment is the records of a transcript a fork carries, in fork order
// (see forkTranscript). Records without a uuid are bookkeeping and dropped.
func forkSegment(data []byte) []map[string]any {
	var recs []map[string]any
	for _, line := range strings.Split(string(data), "\n") {
		var rec map[string]any
		if json.Unmarshal([]byte(strings.TrimSpace(line)), &rec) != nil || rec == nil {
			continue
		}
		if u, _ := rec["uuid"].(string); u == "" {
			continue
		}
		recs = append(recs, rec)
	}
	start := -1
	for i, rec := range recs {
		if rec["type"] == "system" && rec["subtype"] == "compact_boundary" {
			start = i
		}
	}
	if start < 0 {
		return recs
	}
	boundary := recs[start]
	byUUID := map[string]map[string]any{}
	for _, rec := range recs[:start] {
		byUUID[rec["uuid"].(string)] = rec
	}
	var preserved []map[string]any
	for _, u := range preservedUUIDs(boundary) {
		if rec, ok := byUUID[u]; ok {
			preserved = append(preserved, rec)
		}
	}
	after := recs[start+1:]
	summaryAt := -1
	for i, rec := range after {
		if v, _ := rec["isCompactSummary"].(bool); v {
			summaryAt = i
			break
		}
	}
	segment := []map[string]any{boundary}
	head := boundary
	if summaryAt >= 0 {
		segment = append(segment, after[:summaryAt+1]...)
		head = after[summaryAt]
		after = after[summaryAt+1:]
	}
	prev := head["uuid"]
	for _, rec := range preserved {
		rec["parentUuid"] = prev
		prev = rec["uuid"]
		segment = append(segment, rec)
	}
	if len(preserved) > 0 && len(after) > 0 {
		after[0]["parentUuid"] = prev
	}
	return append(segment, after...)
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
