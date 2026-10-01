package runner

import coretranscript "github.com/sloprail/harness-mocks/internal/transcript"

// chainRecord returns line with a uuid (minted if absent) and a parentUuid (set to
// parent if the record carried none), plus the record's resulting uuid so the caller
// can advance the chain. A record that will not parse as a JSON object is returned
// unchanged with an empty uuid. It preserves every field the record already has
// (message/content, sessionId, isSidechain, toolUseResult, …): those are the
// trajectory, and only the identity/link fields are the mock's to complete. An
// explicit null parentUuid (the root) is left as the origin marker.
func chainRecord(line []byte, parent string) (out []byte, uuid string) {
	return coretranscript.Chain(line, parent, chainUUID, chainParent, newRecordUUID)
}

// The fields that chain a Claude Code transcript: each record's own uuid, the
// uuid of the record before it, and the boundary of a compaction's logical parent.
const (
	chainUUID          = "uuid"
	chainParent        = "parentUuid"
	chainLogicalParent = "logicalParentUuid"
)

// asOrigin makes rec parentless — the origin of a chain, which chainRecord leaves
// alone — as a session's root, a sub-agent's dispatch prompt and a compaction
// boundary are.
func asOrigin(rec map[string]any) { rec[chainParent] = nil }

// chainsFromPrevious clears rec's parent, so the record chains from the one
// written before it (the summary after its compaction boundary).
func chainsFromPrevious(rec map[string]any) { delete(rec, chainParent) }

// continuesFrom names the record a parentless boundary continues the chain from.
func continuesFrom(rec map[string]any, uuid string) { rec[chainLogicalParent] = uuid }

// envelope is the bookkeeping Claude Code writes on every record: sessionId,
// cwd, timestamp, isSidechain (and agentId for a sub-agent's), userType,
// entrypoint, version and, inside a repository, gitBranch.
func (st recordStamp) envelope() coretranscript.Envelope {
	e := coretranscript.Envelope{
		"timestamp":   nowStamp(),
		"isSidechain": st.IsSidechain,
		"userType":    stampUserType,
		"entrypoint":  stampEntrypoint,
		"version":     stampVersion,
	}
	if st.SessionID != "" {
		e["sessionId"] = st.SessionID
	}
	if st.Cwd != "" {
		e["cwd"] = st.Cwd
	}
	if st.IsSidechain && st.AgentID != "" {
		e["agentId"] = st.AgentID
	}
	if st.GitBranch != "" {
		e["gitBranch"] = st.GitBranch
	}
	return e
}

// stampRecord fills the bookkeeping fields a real record carries, where line
// does not carry them already. A line that is not a JSON object is returned as
// it is.
//
// sr:provides transcript-record-envelope/claude
func stampRecord(line []byte, st recordStamp) []byte {
	return st.envelope().Stamp(line)
}

// promptMarks is how a `claude -p` prompt record is marked: its source and its
// turn origin are both "sdk".
var promptMarks = map[string]any{"promptSource": "sdk", "turnOrigin": "sdk"}

// markPrompt marks rec as the prompt of a non-interactive run, which every run
// this mock models is.
//
// sr:provides transcript-record-envelope/claude
func markPrompt(rec map[string]any) { coretranscript.MarkPrompt(rec, true, promptMarks) }
