package runner

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"time"

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

// promptCacheLifetime is how long a session's prompt cache lives: the 1-hour cache
// of the recorded runs.
const promptCacheLifetime = time.Hour

// resumeFields is what the SessionStart of a resumed session carries beyond
// source: the time since the transcript's last record, and the cost facts of
// re-sending the context. The mock spends no tokens, so those are 0 (recorded:
// snapshots/runs/forkresume, compact).
func resumeFields(transcriptPath string) *hooks.ResumeFields {
	var last time.Time
	if data, err := os.ReadFile(transcriptPath); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			var rec struct {
				Timestamp string `json:"timestamp"`
			}
			if json.Unmarshal([]byte(line), &rec) == nil {
				if at, err := time.Parse(time.RFC3339Nano, rec.Timestamp); err == nil && at.After(last) {
					last = at
				}
			}
		}
	}
	stats := session.Stats(last, time.Now(), promptCacheLifetime, 0, 0)
	return &hooks.ResumeFields{
		SecondsSinceLastResponse: stats.SecondsSinceLastResponse, ContextTokens: stats.ContextTokens,
		PromptCacheLikelyExpired: stats.CacheLikelyExpired, EstimatedCacheWriteUSD: stats.EstimatedCacheWriteUSD,
	}
}
