package runner

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"

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
	if rec.Type == "system" && rec.Subtype == "api_retry" {
		return true, errAPIRetry
	}
	if rec.Type == "assistant" {
		_, name, _ := extractFirstToolUseWithID(line)
		if refusedTools[name] {
			return true, fmt.Errorf("claude-mock: the %s tool is not implemented by the mock: it is refused rather than ignored", name)
		}
		if name == "AskUserQuestion" && !cfg.PermissionHost {
			return true, fmt.Errorf("claude-mock: the AskUserQuestion tool is offered only to a run with a permission host (--permission-prompt-tool stdio), as a non-interactive claude does: this run has none, so the mock refuses the call rather than ignoring it")
		}
		if name != "" && cfg.RestrictTools && !slices.Contains(cfg.Tools, name) {
			return true, fmt.Errorf("claude-mock: the %s tool is not among the --tools of this run: what claude answers a call to a tool it was not given is not recorded, so the mock refuses it rather than ignoring it", name)
		}
	}
	if err := refuseUnrecordedControl(cfg, inv, rec.Type); err != nil {
		return true, err
	}
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
