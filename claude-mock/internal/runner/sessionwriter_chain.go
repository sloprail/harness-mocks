package runner

import (
	"encoding/json"
)

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
