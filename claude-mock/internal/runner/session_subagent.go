package runner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// subagentMeta is a sub-agent's .meta.json sidecar, in the shape claude
// 2.1.282 writes it (a controlled run of a foreground, a nested and an
// isolated sub-agent; 626 real sidecars): agentType, description, toolUseId,
// parentAgentId for a nested sub-agent, spawnDepth, requestShape
// ("foreground" | "background"), requestNonInteractive (a `-p` session), model
// when the call named one, and, for an isolated sub-agent, worktreePath,
// spawnedWithWorktree and worktreeBranch.
type subagentMeta struct {
	AgentType             string `json:"agentType"`
	WorktreePath          string `json:"worktreePath,omitempty"`
	SpawnedWithWorktree   bool   `json:"spawnedWithWorktree,omitempty"`
	WorktreeBranch        string `json:"worktreeBranch,omitempty"`
	Description           string `json:"description"`
	ToolUseID             string `json:"toolUseId"`
	ParentAgentID         string `json:"parentAgentId,omitempty"`
	SpawnDepth            int    `json:"spawnDepth"`
	RequestShape          string `json:"requestShape"`
	RequestNonInteractive bool   `json:"requestNonInteractive"`
	Model                 string `json:"model,omitempty"`
}

// seedSubagentTranscript writes prompt as the first user record of the
// sub-agent's sidechain transcript at path, and meta as the .meta.json
// sidecar beside it.
//
// The real sub-agent transcript's first record IS the dispatch prompt — a uuid
// with a null parentUuid, the origin every later record chains from — and every
// record carries isSidechain and the file's agentId (controlled 2.1.282 runs
// and every real subagents/agent-<id>.jsonl). A hook can read the prompt (e.g.
// an embedded task id) out of it. subCwd is the sub-agent's cwd (its isolated
// worktree under isolation="worktree"), which the record reports.
//
// sr:docs https://code.claude.com/docs/en/agent-sdk/sessions
func seedSubagentTranscript(path, subCwd, sessionID, agentID, prompt string, meta subagentMeta) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	stamp := newRecordStamp(sessionID, subCwd)
	stamp.IsSidechain, stamp.AgentID = true, agentID
	rec := map[string]any{
		"type":       "user",
		"uuid":       newRecordUUID(),
		"parentUuid": nil,
		"message":    map[string]any{"role": "user", "content": prompt},
	}
	if line, err := marshalRecord(rec); err == nil {
		if f, ferr := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); ferr == nil {
			appendToSession(f, stampRecord(line, stamp))
			f.Close()
		}
	}
	if mb, err := json.Marshal(meta); err == nil {
		_ = os.WriteFile(strings.TrimSuffix(path, ".jsonl")+".meta.json", mb, 0o644)
	}
}
