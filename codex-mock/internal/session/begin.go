package session

import (
	"os"
	"time"

	coresession "github.com/sloprail/harness-mocks/internal/session"
)

// Starts is Codex's SessionStart hook: it fires for every session, and its
// source says how the session began (recorded: runs/session-resume).
var Starts = coresession.StartPolicy{
	Fresh:   coresession.StartHook{Fires: true, Source: "startup"},
	Resumed: coresession.StartHook{Fires: true, Source: "resume"},
}

// Begin is the run's session: a new one under a fresh id, or, when resume names
// a session, that session's own rollout and id, whatever directory it is
// resumed in.
//
// sr:provides session-resume/codex
func Begin(home, cwd, resume string, now time.Time) (id string, rollout *File, resumed bool, err error) {
	if resume != "" {
		rollout, err = Open(home, resume)
		return resume, rollout, true, err
	}
	id = coresession.NewID()
	rollout, err = Create(home, id, cwd, now)
	return id, rollout, false, err
}

// Open continues session id in its existing rollout, wherever it was begun
// (`codex exec resume <id>` finds it by id, from any working directory,
// recorded: runs/session-resume): the new records are appended to the same
// file, which keeps its meta record.
//
// sr:provides session-resume/codex
func Open(home, id string) (*File, error) {
	path := findRollout(home, id)
	if path == "" {
		return nil, &ErrNoRollout{ID: id}
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	return &File{Path: path, f: f}, nil
}
