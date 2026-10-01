package runner

import (
	"context"
	"fmt"
	"strings"

	"github.com/sloprail/harness-mocks/claude-mock/internal/hooks"
)

// handleControlRecord checks whether rec is a mock-only control record.
// If it is, the appropriate hook is fired and (true, err) is returned so the
// caller can skip forwarding the line to stdout.
// If rec is a normal stream-json record, (false, nil) is returned.
//
// Control record types:
//   - worktree_create / worktree_remove — fire WorktreeCreate / WorktreeRemove
//   - subagent_start                    — fire SubagentStart with explicit agent_type
//
// sr:docs https://docs.anthropic.com/en/docs/claude-code/hooks#worktreecreate
// sr:docs https://docs.anthropic.com/en/docs/claude-code/hooks#worktreeremove
// sr:docs https://docs.anthropic.com/en/docs/claude-code/hooks#subagentstart
// sr:invariant control-records
func handleControlRecord(ctx context.Context, rec *cliRecord, line []byte, cfg Config, inv *hooks.Invoker, tr *transcript) (handled bool, err error) {
	switch rec.Type {
	case "worktree_create", "worktree_remove":
		evt := hooks.EventWorktreeCreate
		if rec.Type == "worktree_remove" {
			evt = hooks.EventWorktreeRemove
		}
		if _, err := inv.Fire(ctx, hooks.Input{
			SessionID:     cfg.SessionID,
			Cwd:           cfg.Cwd,
			HookEventName: evt,
			WorktreeName:  rec.WorktreeName,
		}); err != nil {
			fmt.Fprintf(cfg.Stderr, "claude-mock: %s hook blocked: %v\n", evt, err)
			return true, err
		}
		return true, nil

	case "subagent_start":
		if _, err := inv.Fire(ctx, hooks.Input{
			SessionID:     cfg.SessionID,
			Cwd:           cfg.Cwd,
			HookEventName: hooks.EventSubagentStart,
			AgentType:     rec.AgentType,
		}); err != nil {
			fmt.Fprintf(cfg.Stderr, "claude-mock: SubagentStart hook blocked: %v\n", err)
			return true, err
		}
		return true, nil
	}

	return false, nil
}

// defaultPreserved is how many of the last records a compaction keeps when
// the scenario does not say. Real compactions kept between 2 and 21 (every
// compact_boundary in the real transcripts); a manual /compact in a controlled
// claude 2.1.282 run kept 2.
const defaultPreserved = 2

// compactHookLines is what a compaction reports of its Pre/PostCompact
// hooks, one line per hook, in the 2.1.282 binary's wording:
// "<Event> [<command>] completed successfully[: <output>]" or
// "<Event> [<command>] failed[: <output>]".
func compactHookLines(event string, runs []hooks.HandlerRun) []string {
	var out []string
	for _, r := range runs {
		verdict, text := "completed successfully", strings.TrimSpace(r.Stdout)
		if r.ExitCode != 0 {
			verdict, text = "failed", strings.TrimSpace(r.Stderr)
		}
		l := event + " [" + r.Command + "] " + verdict
		if text != "" {
			l += ": " + text
		}
		out = append(out, l)
	}
	return out
}

// compactionSpec is what one compaction's boundary records.
type compactionSpec struct {
	logicalParent string
	preserve      int
	anchor        string
	trigger       string
	preTokens     int
	postTokens    int
	durationMs    int64
	withSegment   bool
	tailOffset    int
}
