package session

import (
	"encoding/json"
	"strings"
)

// Record is one transcript record, a JSON object.
type Record = map[string]any

// ForkSchema is how a harness's records show what a fork needs to read.
type ForkSchema struct {
	// UUID and Parent are the fields that identify a record and chain it to the
	// record before; SessionKey is the field naming the session it belongs to.
	UUID, Parent, SessionKey string
	// IsBoundary marks a compaction boundary, IsSummary the summary that follows
	// it, and Preserved lists the records a boundary says the compaction kept.
	IsBoundary, IsSummary func(Record) bool
	Preserved             func(boundary Record) []string
}

// ParseRecords is the records of a JSONL transcript that carry a uuid, in
// order; records without one are bookkeeping and dropped.
func ParseRecords(data []byte, uuidKey string) []Record {
	var recs []Record
	for _, line := range strings.Split(string(data), "\n") {
		var rec Record
		if json.Unmarshal([]byte(strings.TrimSpace(line)), &rec) != nil || rec == nil {
			continue
		}
		if u, _ := rec[uuidKey].(string); u == "" {
			continue
		}
		recs = append(recs, rec)
	}
	return recs
}

// Fork is the records a fork of a session carries under the new session id
// newID. A session never compacted forks whole, every record with its parent
// unchanged. A compacted session forks from its last compaction boundary: the
// boundary, what follows it up to and including the summary, the records the
// boundary lists as preserved re-parented into one chain after the summary, then
// everything after the summary, the first of it re-parented onto the last
// preserved record. The source is only read: recs are what the caller parsed of
// it, and Fork changes only the fork's own copies of them.
//
// sr:capability session-fork
func Fork(recs []Record, newID string, s ForkSchema) []Record {
	segment := forkSegment(recs, s)
	for _, rec := range segment {
		rec[s.SessionKey] = newID
	}
	return segment
}

func forkSegment(recs []Record, s ForkSchema) []Record {
	start := -1
	for i, rec := range recs {
		if s.IsBoundary(rec) {
			start = i
		}
	}
	if start < 0 {
		return recs
	}
	boundary := recs[start]
	byUUID := map[string]Record{}
	for _, rec := range recs[:start] {
		byUUID[rec[s.UUID].(string)] = rec
	}
	var preserved []Record
	for _, u := range s.Preserved(boundary) {
		if rec, ok := byUUID[u]; ok {
			preserved = append(preserved, rec)
		}
	}
	after := recs[start+1:]
	summaryAt := -1
	for i, rec := range after {
		if s.IsSummary(rec) {
			summaryAt = i
			break
		}
	}
	segment := []Record{boundary}
	head := boundary
	if summaryAt >= 0 {
		segment = append(segment, after[:summaryAt+1]...)
		head = after[summaryAt]
		after = after[summaryAt+1:]
	}
	prev := head[s.UUID]
	for _, rec := range preserved {
		rec[s.Parent] = prev
		prev = rec[s.UUID]
		segment = append(segment, rec)
	}
	if len(preserved) > 0 && len(after) > 0 {
		after[0][s.Parent] = prev
	}
	return append(segment, after...)
}
