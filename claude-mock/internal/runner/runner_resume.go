package runner

import (
	"context"
	"errors"

	"github.com/sloprail/harness-mocks/claude-mock/internal/hooks"
	"github.com/sloprail/harness-mocks/internal/session"
)

// resumeFailed is the error `claude` ends a run with when --resume names a
// session no transcript holds (see ErrNoConversation), after firing the hooks of
// the phases the session capability says still fire (SessionEnd, never
// SessionStart); nil when the session exists.
//
// sr:provides session-resume-unknown/claude
// sr:docs https://code.claude.com/docs/en/cli-reference#cli-flags
func resumeFailed(ctx context.Context, cfg Config, settings *hooks.Settings, from string) error {
	_, err := session.Resume(claudeLayout, cfg.ConfigDir, cfg.Cwd, from)
	var nc *session.NoConversationError
	if !errors.As(err, &nc) {
		return nil
	}
	inv := hooks.NewInvoker(settings, cfg.Cwd, from)
	inv.SetTranscriptPath(sessionFilePath(cfg.ConfigDir, cfg.Cwd, from))
	for _, phase := range nc.Fire {
		if phase == session.End {
			fireSessionEnd(ctx, cfg, inv)
		}
	}
	return &ErrNoConversation{SessionID: nc.SessionID}
}
