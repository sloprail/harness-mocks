package session

import (
	"fmt"
	"os"
	"time"

	coresession "github.com/sloprail/harness-mocks/internal/session"
)

// Fork starts the rollout of session id as a fork of session from, what
// `codex exec fork <from>` leaves (recorded: runs/session-fork). The fork has
// its own rollout, under its own id, and that file does not copy the history:
// its meta record names the session it was forked from and where in it
// (forked_from_id, forked_from_ordinal_exclusive and history_base), and holds
// only what is said after. The source rollout is only read.
//
// The scripted agent is handed this rollout, so it sees only what is said in
// the fork, not the history it points at.
//
// sr:provides session-fork/codex
func Fork(home, from, id, cwd string, now time.Time) (*File, error) {
	src := findRollout(home, from)
	if src == "" {
		return nil, fmt.Errorf("no saved session found with ID %s", from)
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return nil, fmt.Errorf("fork: %w", err)
	}
	base := coresession.ForkBase(from, data)
	return create(home, id, cwd, now, map[string]any{
		"forked_from_id": from, "forked_from_ordinal_exclusive": base.Records, "thread_source": "user",
		"history_base": map[string]any{"thread_id": base.From, "end_ordinal_exclusive": base.Records, "end_byte_offset": base.Bytes},
	})
}
