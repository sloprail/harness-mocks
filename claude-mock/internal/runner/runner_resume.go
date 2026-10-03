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

// bytesPerToken and cacheWriteUSDPerMillionTokens are how the mock estimates what
// re-sending a session's context costs: the mock counts no tokens, so the
// context is its transcript's size over a rough four bytes a token, priced at
// the rate every recorded resume shows (estimated_cache_write_usd is 2 dollars
// per million context_tokens: snapshots/runs/forkresume, compact, resume-continue).
const (
	bytesPerToken                 = 4
	cacheWriteUSDPerMillionTokens = 2.0
)

// resumeFields is what the SessionStart of a resumed session carries beyond
// source: the time since the transcript's last record, and the cost facts of
// re-sending the context (an estimate, see bytesPerToken).
func resumeFields(transcriptPath string) *hooks.ResumeFields {
	var last time.Time
	contextTokens, title := 0, ""
	if data, err := os.ReadFile(transcriptPath); err == nil {
		contextTokens, title = len(data)/bytesPerToken, transcriptAgentName(data)
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
	stats := session.Stats(last, time.Now(), promptCacheLifetime, contextTokens, cacheWriteUSDPerMillionTokens)
	return &hooks.ResumeFields{
		SessionTitle: title, SecondsSinceLastResponse: stats.SecondsSinceLastResponse, ContextTokens: stats.ContextTokens,
		PromptCacheLikelyExpired: stats.CacheLikelyExpired, EstimatedCacheWriteUSD: stats.EstimatedCacheWriteUSD,
	}
}

// writePrompt writes the prompt as the HUMAN turn it is, AFTER SessionStart and
// after UserPromptSubmit has let it through (a transcript is not even created
// before then: recorded, snapshots/runs/transcript-at-start; a refused prompt
// leaves only its warning, snapshots/runs/prompt-blocked):
//
//   - FRESH: if SessionStart printed anything its attachment is already the
//     file's origin and the prompt chains after it; if not, the prompt is the
//     first record and so the origin itself. Its uuid is the deterministic
//     `e2e-root-<session>` either way, so a caller can reference the human
//     message up front.
//   - RESUME / FORK: the NEXT human turn, chained into the transcript on disk,
//     never a second parentless root.
//   - NESTED sub-agent run: nothing. The sub-agent's human-origin record is its
//     dispatch prompt, seeded into its sidechain file by prepareSubagent; writing
//     it again would forge a human message the user never sent.
func writePrompt(tr *transcript, cfg Config, nested bool) {
	switch {
	case nested:
	case cfg.IsResume:
		appendResumePrompt(tr, cfg.SessionID, cfg.Prompt)
	default:
		writeRootPrompt(tr, cfg.SessionID, cfg.Cwd, cfg.Prompt)
	}
}

// ErrNoConversation is --resume naming a session that has no transcript. Real
// Claude Code prints "No conversation found with session ID: <id>" and exits 1
// (claude 2.1.282; a stream-json run also writes an error result frame).
type ErrNoConversation struct{ SessionID string }

func (e *ErrNoConversation) Error() string {
	return "No conversation found with session ID: " + e.SessionID
}
