// Package compaction is the harness-neutral core of compacting a session: the
// sequence of a compaction and what its boundary preserves.
package compaction

// PlanInput is what a compaction's boundary is planned from.
type PlanInput struct {
	// WithSegment is whether the compaction keeps a tail of earlier records.
	WithSegment bool
	// Preserve is how many written records the kept tail holds, and TailOffset
	// how many records before the boundary the tail ends (0: the last written).
	Preserve, TailOffset int
	// LogicalParent overrides the boundary's logical parent: "" for the default
	// (the tail's last record, else the last written one), Unwritten for a record
	// that was never written, anything else as given.
	LogicalParent string
	// Written is the uuids of the last n records written, oldest first.
	Written func(n int) []string
	// LastUUID is the last record written.
	LastUUID string
	// IsWritten reports whether a record is in the transcript.
	IsWritten func(uuid string) bool
	// NewUUID mints a record id.
	NewUUID func() string
}

// Unwritten is a LogicalParent naming a record that was never written.
const Unwritten = "unwritten"

// Plan is what a compaction's boundary records: the kept tail, the record the
// boundary continues from, and every id the boundary accounts for.
type Plan struct {
	// Kept is the preserved tail, oldest first; empty without a segment.
	Kept []string
	// LogicalParent is the record the boundary continues the chain from: the
	// tail's last record wherever it is written, so a reader can still walk the
	// chain across the boundary.
	LogicalParent string
	// All is Kept plus the logical parent when it is neither kept nor written.
	All []string
}

// PlanBoundary plans a compaction's boundary so the record chain stays
// walkable across it: a boundary, then a summary, then a preserved tail of
// earlier records, the boundary's logical parent being that tail's last record.
//
// sr:capability compaction-transcript-continuity
func PlanBoundary(in PlanInput) Plan {
	kept := []string{}
	if in.WithSegment {
		window := in.Written(in.Preserve + in.TailOffset)
		if in.TailOffset > 0 {
			if len(window) > in.TailOffset {
				window = window[:len(window)-in.TailOffset]
			} else {
				window = []string{}
			}
		}
		kept = window
	}
	logicalParent := in.LogicalParent
	switch logicalParent {
	case "":
		if len(kept) > 0 {
			logicalParent = kept[len(kept)-1]
		} else {
			logicalParent = in.LastUUID
		}
	case Unwritten:
		logicalParent = in.NewUUID()
	}
	all := append([]string(nil), kept...)
	if logicalParent != "" && !contains(kept, logicalParent) && !in.IsWritten(logicalParent) {
		all = append(all, logicalParent)
	}
	return Plan{Kept: kept, LogicalParent: logicalParent, All: all}
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
