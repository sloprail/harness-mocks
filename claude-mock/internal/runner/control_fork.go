package runner

import (
	"encoding/json"
	"strings"
)

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
