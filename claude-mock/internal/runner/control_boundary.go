package runner

import (
	"encoding/json"
	"os"
	"strings"
)

// writeCompactBoundary appends the compact_boundary a compaction opens with.
// It is APPENDED to the file the session is writing, not a new file: the file
// a real compaction happened in holds its boundary part-way down (46 of the 65
// real boundaries), and only a later resume moves the conversation to a new
// file.
func writeCompactBoundary(tr *transcript, spec compactionSpec) {
	// Two shapes, both observed in manual compactions of claude 2.1.282:
	//
	//   - with a preserved segment (F:compact; 64 of 65 real boundaries):
	//     compactMetadata names the kept records — preservedMessages {anchorUuid,
	//     uuids, allUuids} and preservedSegment {headUuid, anchorUuid, tailUuid},
	//     uuids being N consecutive written records (see the tail below). allUuids is uuids plus records
	//     never written to the file: a strict superset in 44 of 65 real
	//     boundaries, every extra id unwritten.
	//   - without one (F:compact-nohooks; 1 of 65): neither field.
	//
	// The logical parent is the preserved segment's TAIL — uuids' last id —
	// wherever it is written: all 55 real boundaries whose logical parent is a
	// written record. The tail sits in three places (66 real boundaries):
	//
	//   - the record written immediately before the boundary: 34 (33
	//     automatic, 1 manual), and F:compact-nohooks. This is the default.
	//   - an EARLIER written record, the segment ending 2 to 253 records
	//     before the boundary: 7 mid-file boundaries. "tail_offset": K ends the
	//     segment K records back.
	//   - a record copied in AFTER the boundary: the 13 fork files, and one
	//     mid-file boundary. forkTranscript writes this form.
	//
	// The remaining 11 real boundaries (all automatic) and the manual
	// F:compact name a record never written, which then closes allUuids:
	// "logical_parent":"unwritten". Any other logical_parent is used as given.
	withSegment := spec.withSegment
	kept := []string{}
	if withSegment {
		window := tr.lastUUIDs(spec.preserve + spec.tailOffset)
		if spec.tailOffset > 0 {
			if len(window) > spec.tailOffset {
				window = window[:len(window)-spec.tailOffset]
			} else {
				window = []string{}
			}
		}
		kept = window
	}
	logicalParent := spec.logicalParent
	switch logicalParent {
	case "":
		if len(kept) > 0 {
			logicalParent = kept[len(kept)-1]
		} else {
			logicalParent = tr.lastUUID()
		}
	case "unwritten":
		logicalParent = newRecordUUID()
	}
	all := append([]string(nil), kept...)
	if logicalParent != "" && !contains(kept, logicalParent) && !fileHasUUID(tr.path, logicalParent) {
		all = append(all, logicalParent)
	}
	meta := map[string]any{
		"trigger":    spec.trigger,
		"preTokens":  spec.preTokens,
		"durationMs": spec.durationMs,
	}
	if withSegment && len(kept) > 0 {
		meta["preservedSegment"] = map[string]any{
			"headUuid": kept[0], "anchorUuid": spec.anchor, "tailUuid": kept[len(kept)-1],
		}
		meta["preservedMessages"] = map[string]any{
			"anchorUuid": spec.anchor, "uuids": kept, "allUuids": all,
		}
	}
	meta["postTokens"] = spec.postTokens
	// cumulativeDroppedTokens: what the session's compactions have dropped so
	// far, this one's preTokens - postTokens included (both manual fixtures:
	// 23138-2308 = 20830, 22932-2334 = 20598). 2.1.282 always writes it; 13
	// of the 66 real boundaries, from older versions, lack it.
	meta["cumulativeDroppedTokens"] = lastCumulativeDropped(tr.path) + spec.preTokens - spec.postTokens
	tr.persistMap(map[string]any{
		"parentUuid":        nil,
		"logicalParentUuid": logicalParent,
		"type":              "system",
		"subtype":           "compact_boundary",
		"content":           "Conversation compacted",
		"isMeta":            false,
		"level":             "info",
		"compactMetadata":   meta,
	})
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
