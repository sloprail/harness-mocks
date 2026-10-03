package session

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Origin is where a sub-agent's thread comes from: the session it belongs to,
// the thread that spawned it and how many layers deep it is.
type Origin struct {
	Session, Parent string
	Depth           int
}

// CreateSub starts the rollout of a sub-agent's thread: a file of its own in
// the same day directory as the session's (so all of a session's threads sit
// in one directory, named as Create names it), whose meta record names the
// thread that spawned it and its depth (recorded: runs/nested-subagents).
func CreateSub(home, id, cwd string, now time.Time, o Origin) (*File, error) {
	dir := filepath.Join(home, "sessions", now.Format("2006"), now.Format("01"), now.Format("02"))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, fmt.Sprintf("rollout-%s-%s.jsonl", now.Format("2006-01-02T15-04-05"), id))
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	s := &File{Path: path, f: f}
	s.append("session_meta", map[string]any{"id": id, "session_id": o.Session, "parent_thread_id": o.Parent,
		"cwd": cwd, "originator": "codex_exec", "cli_version": "mock", "thread_source": "subagent",
		"source": map[string]any{"subagent": map[string]any{"thread_spawn": map[string]any{
			"parent_thread_id": o.Parent, "depth": o.Depth, "agent_path": nil, "agent_nickname": nil, "agent_role": nil}}}})
	return s, nil
}
