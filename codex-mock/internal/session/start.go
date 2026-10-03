package session

import (
	"time"

	coresession "github.com/sloprail/harness-mocks/internal/session"
)

// forked is the start hook of a fork: it fires with source "fork" (recorded:
// runs/session-fork).
var forked = coresession.StartHook{Fires: true, Source: "fork"}

// Start is the run's session and its start hook: the fork of session forkFrom
// when one is named, else Begin's session (new, or resumed by its id).
func Start(home, cwd, resume, forkFrom string, now time.Time) (id string, rollout *File, start coresession.StartHook, err error) {
	if forkFrom == "" {
		var resumed bool
		id, rollout, resumed, err = Begin(home, cwd, resume, now)
		return id, rollout, Starts.For(resumed), err
	}
	id = coresession.NewID()
	rollout, err = Fork(home, forkFrom, id, cwd, now)
	return id, rollout, forked, err
}
