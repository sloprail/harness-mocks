package runner

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/sloprail/harness-mocks/claude-mock/internal/hooks"
	"github.com/sloprail/harness-mocks/internal/subagents"
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
// A failing create hook aborts the creation (subagents.WorktreeHook).
//
// sr:provides worktree-hooks/claude
// sr:invariant control-records
func handleControlRecord(ctx context.Context, rec *cliRecord, line []byte, cfg Config, inv *hooks.Invoker, tr *transcript) (handled bool, err error) {
	switch rec.Type {
	case "worktree_create", "worktree_remove":
		evt := hooks.EventWorktreeCreate
		in := hooks.Input{SessionID: cfg.SessionID, Cwd: cfg.Cwd, HookEventName: evt, WorktreeName: rec.WorktreeName}
		if rec.Type == "worktree_remove" {
			evt = hooks.EventWorktreeRemove
			in = hooks.Input{SessionID: cfg.SessionID, Cwd: cfg.Cwd, HookEventName: evt, WorktreePath: rec.WorktreePath}
			if in.WorktreePath == "" {
				in.WorktreePath = filepath.Join(cfg.Cwd, ".claude", "worktrees", rec.WorktreeName)
			}
		}
		if _, _, err := subagents.WorktreeHook(false, func() (string, bool, error) {
			_, err := inv.Fire(ctx, in)
			return "", true, err
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
