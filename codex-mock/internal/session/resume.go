package session

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	coresession "github.com/sloprail/harness-mocks/internal/session"
)

// findRollout is the rollout of session id under <home>/sessions, whichever
// day it was begun: "" when there is none, and for an id that is not a plain
// name.
func findRollout(home, id string) string {
	if id == "" || strings.ContainsAny(id, `/\*?[`) {
		return ""
	}
	hits, _ := filepath.Glob(filepath.Join(home, "sessions", "*", "*", "*", "rollout-*-"+id+".jsonl"))
	for _, p := range hits {
		if fi, err := os.Stat(p); err == nil && fi.Mode().IsRegular() {
			return p
		}
	}
	return ""
}

// ErrNoRollout is `codex exec resume <id>` naming a session that has no
// rollout: it fails with this message before any session starts, so no hook
// fires at all, neither its start nor its end (recorded:
// snapshots/runs/session-resume-unknown).
type ErrNoRollout struct{ ID string }

func (e *ErrNoRollout) Error() string {
	return fmt.Sprintf("Error: thread/resume: thread/resume failed: no rollout found for thread id %s (code -32600)", e.ID)
}

// ResumeUnknown is the failure of resuming session id, or nil when its rollout
// exists. No hook phase fires on it.
// sr:provides session-resume-unknown/codex
func ResumeUnknown(home, id string) error {
	_, err := coresession.ResumeAt(findRollout(home, id), id)
	var nc *coresession.NoConversationError
	if errors.As(err, &nc) {
		return &ErrNoRollout{ID: nc.SessionID}
	}
	return nil
}
