package runner

import (
	"encoding/json"
	"os"
	"strings"

	"github.com/sloprail/harness-mocks/internal/compaction"
)

// writeCompactBoundary appends the compact_boundary a compaction opens with and
// returns the writer of the stream frames that announce it.
// It is APPENDED to the file the session is writing, not a new file: the file
// a real compaction happened in holds its boundary part-way down (46 of the 65
// real boundaries), and only a later resume moves the conversation to a new
// file.
//
// What the boundary preserves (the kept tail, the logical parent, the ids it
// accounts for) is the compaction capability's plan (compaction.PlanBoundary);
// this writes it in Claude Code's compactMetadata shape. Two shapes, both
// observed in manual compactions of claude 2.1.282:
//
//   - with a preserved segment (F:compact; 64 of 65 real boundaries):
//     compactMetadata names the kept records — preservedMessages {anchorUuid,
//     uuids, allUuids} and preservedSegment {headUuid, anchorUuid, tailUuid}.
//     allUuids is uuids plus records never written to the file: a strict
//     superset in 44 of 65 real boundaries, every extra id unwritten.
//   - without one (F:compact-nohooks; 1 of 65): neither field.
//
// sr:provides compaction-transcript-continuity/claude
func writeCompactBoundary(cfg Config, tr *transcript, spec compactionSpec) (streamFrames func()) {
	plan := compaction.PlanBoundary(compaction.PlanInput{
		WithSegment: spec.withSegment, Preserve: spec.preserve, TailOffset: spec.tailOffset,
		LogicalParent: spec.logicalParent,
		Written:       tr.lastUUIDs, LastUUID: tr.lastUUID(),
		IsWritten: func(uuid string) bool { return fileHasUUID(tr.path, uuid) },
		NewUUID:   newRecordUUID,
	})
	meta := map[string]any{
		"trigger":    spec.trigger,
		"preTokens":  spec.preTokens,
		"durationMs": spec.durationMs,
	}
	if spec.withSegment && len(plan.Kept) > 0 {
		meta["preservedSegment"] = map[string]any{
			"headUuid": plan.Kept[0], "anchorUuid": spec.anchor, "tailUuid": plan.Kept[len(plan.Kept)-1],
		}
		meta["preservedMessages"] = map[string]any{
			"anchorUuid": spec.anchor, "uuids": plan.Kept, "allUuids": plan.All,
		}
	}
	meta["postTokens"] = spec.postTokens
	// cumulativeDroppedTokens: what the session's compactions have dropped so
	// far, this one's preTokens - postTokens included (both manual fixtures:
	// 23138-2308 = 20830, 22932-2334 = 20598). 2.1.282 always writes it; 13
	// of the 66 real boundaries, from older versions, lack it.
	meta["cumulativeDroppedTokens"] = lastCumulativeDropped(tr.path) + spec.preTokens - spec.postTokens
	boundary := map[string]any{
		"type":            "system",
		"subtype":         "compact_boundary",
		"content":         "Conversation compacted",
		"isMeta":          false,
		"level":           "info",
		"compactMetadata": meta,
	}
	asOrigin(boundary)
	continuesFrom(boundary, plan.LogicalParent)
	tr.persistMap(boundary)
	// The stream's frames for it come later: SessionStart:compact streams its
	// hook frames before the end status, init and boundary (recorded:
	// snapshots/runs/compact).
	return func() { writeCompactedFrames(cfg, meta, plan.LogicalParent) }
}

// lastCumulativeDropped is the cumulativeDroppedTokens of the last compact
// boundary in path, or 0.
func lastCumulativeDropped(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	last := 0
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.Contains(line, "compact_boundary") {
			continue
		}
		var rec struct {
			Subtype string `json:"subtype"`
			Meta    struct {
				Dropped int `json:"cumulativeDroppedTokens"`
			} `json:"compactMetadata"`
		}
		if json.Unmarshal([]byte(line), &rec) == nil && rec.Subtype == "compact_boundary" {
			last = rec.Meta.Dropped
		}
	}
	return last
}
