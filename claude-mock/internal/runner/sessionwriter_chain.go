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
	return coretranscript.Chain(line, parent, "uuid", "parentUuid", newRecordUUID)
}

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
