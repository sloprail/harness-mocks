package runner

import (
	"encoding/json"
	"os"
	"strings"
)

// sessionWriter persists JSONL records to the transcript FILE the way real Claude
// Code writes it: every persisted record carries a uuid and a NON-null parentUuid
// that chains it to the record written before it. It wraps the open session file
// and the uuid of the last record it persisted, so the chain threads across the
// whole run (every turn, every synthesised tool_result) without every call site
// having to carry the "last uuid" itself.
//
// # Why this exists — matching the real transcript
//
// Verified against a real ~/.claude/projects/<proj>/<session>.jsonl: an assistant
// record carries {uuid, parentUuid(non-null), sessionId, isSidechain, …} and a
// tool_result record (type:"user") likewise carries uuid + a non-null parentUuid.
// EVERY record chains from the one before it; the only parentless record is the
// conversation's ROOT (uuid + parentUuid null), and the only uuid-less records are
// the preamble/bookkeeping lines (custom-title / ai-title / mode / last-prompt /
// queue-operation / pr-link). The mock previously appended its streamed
// assistant/tool_result/result records with NO uuid at all, so a consumer that keys
// entries on uuid presence (the identity walk, trajectory normalize) saw only the
// seeded root as an entry and dropped every turn — a divergence from real CC.
//
// # What it does NOT touch
//
//   - The STDOUT stream. Only the FILE is chained here; cfg.Out.Write stays exactly
//     as the mock's `claude`-compatible stream (sr-agent and tests read it and it
//     must not change).
//   - A record's EXISTING uuid. A scenario that already stamped a uuid on a record
//     (the sloprail harness stamps `e2e-turn-<id>`; a mock test stamps its own) owns
//     that record's identity — real CC assigns a uuid once, and re-minting here would
//     serve no fidelity and would move an id a test may reference. So a uuid is MINTED
//     only when the record has none (the synthesised tool_result, a mock-test
//     assistant line written without one). The parent chain is likewise filled only
//     when the record does not already carry a non-null parentUuid.
//   - The ROOT's parentlessness. A record that arrives with an explicit null
//     parentUuid (the seeded root) is left parentless — the identity walk finds the
//     conversation origin by "first uuid-carrying record whose parentUuid is null", so
//     forcing a parent onto the root would hide the origin. Only records with an
//     ABSENT parentUuid are chained.
type sessionWriter struct {
	f        *os.File
	lastUUID string

	// stamp is filled onto every persisted record that lacks the field — the
	// bookkeeping every real record carries (sessionId, cwd, timestamp, and for
	// a sub-agent's sidechain file isSidechain + agentId).
	stamp recordStamp
}

// newSessionWriter builds a writer over the session file, seeding the chain from the
// uuid of the LAST record already on disk (so a resume, or a nested sub-agent run
// re-opening the parent transcript, continues the existing chain rather than starting
// a new one). An empty seed is fine: the first chained record then simply mints a uuid
// and, lacking a parent to point at, is left as-is by chain() — the seeded root is that
// first record and must stay parentless anyway.
func newSessionWriter(f *os.File) *sessionWriter {
	w := &sessionWriter{f: f}
	if f != nil {
		w.lastUUID = lastRecordUUID(f.Name())
	}
	return w
}

// persist writes one record to the session file with the real-CC chaining applied:
// a uuid is minted if the record lacks one, a non-null parentUuid is filled from the
// last persisted record if the record has none, and the last-uuid cursor advances to
// this record's uuid. The line is re-serialised only when a field was added; an
// already-complete record (or an unparseable one) is written through untouched.
func (w *sessionWriter) persist(line []byte) {
	if w == nil || w.f == nil {
		return
	}
	chained, uuid := chainRecord(line, w.lastUUID)
	chained = stampRecord(chained, w.stamp)
	appendToSession(w.f, chained)
	if uuid != "" {
		w.lastUUID = uuid
	}
}

// chainRecord returns line with a uuid (minted if absent) and a parentUuid (set to
// parent if the record carried none), plus the record's resulting uuid so the caller
// can advance the chain. A record that will not parse as a JSON object is returned
// unchanged with an empty uuid — the mock forwards malformed lines rather than failing,
// and a line that is not an object cannot carry a chain anyway.
//
// The transformation is deliberately minimal: it decodes into a generic map, adds only
// the missing keys, and re-marshals. It preserves every field the record already has
// (message/content, sessionId, isSidechain, toolUseResult, …) because those are the
// trajectory, and only the identity/link fields are the mock's to complete.
func chainRecord(line []byte, parent string) (out []byte, uuid string) {
	var rec map[string]any
	if err := json.Unmarshal(line, &rec); err != nil || rec == nil {
		return line, ""
	}
	changed := false

	// uuid: keep an existing non-empty one; mint otherwise.
	if s, ok := rec["uuid"].(string); ok && s != "" {
		uuid = s
	} else {
		uuid = newRecordUUID()
		if uuid != "" {
			rec["uuid"] = uuid
			changed = true
		}
	}

	// parentUuid: fill only when ABSENT. An explicit null (the root) is left as the
	// origin marker; an existing non-null parent (a scenario that authored its own
	// chain) is honoured. The FIRST record of a file, with nothing to chain from,
	// gets an explicit null — the way real Claude Code writes an origin.
	if _, present := rec["parentUuid"]; !present {
		if parent != "" {
			rec["parentUuid"] = parent
		} else {
			rec["parentUuid"] = nil
		}
		changed = true
	}

	if !changed {
		return line, uuid
	}
	if b, err := marshalRecord(rec); err == nil {
		return b, uuid
	}
	return line, uuid
}

// lastRecordUUID returns the uuid of the last record in the transcript at path that
// carries one, or "" when the file is missing, empty, or holds only uuid-less records.
// It is the chain seed for a run that opens a transcript with history already in it.
func lastRecordUUID(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	last := ""
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var rec struct {
			UUID string `json:"uuid"`
		}
		if json.Unmarshal([]byte(line), &rec) != nil {
			continue
		}
		if rec.UUID != "" {
			last = rec.UUID
		}
	}
	return last
}

// stampRecord fills the bookkeeping fields a real record carries, where line
// does not carry them already. A line that is not a JSON object is returned as
// it is.
func stampRecord(line []byte, st recordStamp) []byte {
	var rec map[string]any
	if err := json.Unmarshal(line, &rec); err != nil || rec == nil {
		return line
	}
	changed := false
	set := func(k string, v any) {
		if _, ok := rec[k]; !ok {
			rec[k] = v
			changed = true
		}
	}
	if st.SessionID != "" {
		set("sessionId", st.SessionID)
	}
	if st.Cwd != "" {
		set("cwd", st.Cwd)
	}
	set("timestamp", nowStamp())
	set("isSidechain", st.IsSidechain)
	if st.IsSidechain && st.AgentID != "" {
		set("agentId", st.AgentID)
	}
	set("userType", stampUserType)
	set("entrypoint", stampEntrypoint)
	set("version", stampVersion)
	if st.GitBranch != "" {
		set("gitBranch", st.GitBranch)
	}
	if !changed {
		return line
	}
	if b, err := marshalRecord(rec); err == nil {
		return b
	}
	return line
}
