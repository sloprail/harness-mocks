package runner

import (
	"context"
	"errors"
	"fmt"

	"github.com/sloprail/harness-mocks/claude-mock/internal/hooks"
	"github.com/sloprail/harness-mocks/claude-mock/internal/toolexec"
	corehooks "github.com/sloprail/harness-mocks/internal/hooks"
)

// decidePreTool records on pending what the PreToolUse hooks decided: refused
// by an exit 2 (quoting the command and its stderr) or by a JSON deny (its
// reason). Any other hook error is returned.
func decidePreTool(cfg Config, pending *pendingToolUse, hookOut hooks.Output, hookErr error) error {
	var blockErr *hooks.BlockError
	blocked := errors.As(hookErr, &blockErr)
	if hookErr != nil && !blocked {
		return hookErr
	}
	blockReason := ""
	if blocked {
		fmt.Fprintf(cfg.Stderr, "claude-mock: PreToolUse hook blocked: %v\n", hookErr)
		blockReason = blockErr.Quoted()
	}
	// sr:provides pretooluse-refusal/claude
	pending.Blocked, pending.BlockReason = corehooks.PreToolDecision(blocked, blockReason, isDeny(hookOut), denyReason(hookOut))
	return nil
}

// firePostTool fires the hook a tool call's result calls for: PostToolUse
// with the tool's response, or PostToolUseFailure with the error text the
// agent got, or neither.
func firePostTool(ctx context.Context, cfg Config, inv *hooks.Invoker, pending pendingToolUse, res toolexec.Result, took int64) {
	outcome := corehooks.ToolSucceeded
	switch {
	case res.Failed:
		outcome = corehooks.ToolFailed
	case res.IsError:
		outcome = corehooks.ToolErrored
	}
	// sr:provides tool-failure-hook/claude
	switch corehooks.AfterToolHook(outcome) {
	case corehooks.AfterFailure:
		notInterrupted := false
		_, _ = inv.Fire(ctx, hooks.Input{
			SessionID:     cfg.SessionID,
			Cwd:           cfg.Cwd,
			HookEventName: hooks.EventPostToolUseFailure,
			ToolName:      pending.ToolName,
			ToolUseID:     pending.ToolUseID,
			ToolInput:     pending.ToolInput,
			Error:         res.Output,
			IsInterrupt:   &notInterrupted,
			DurationMs:    &took,
		})
	case corehooks.AfterSuccess:
		_, _ = inv.Fire(ctx, hooks.Input{
			SessionID:     cfg.SessionID,
			Cwd:           cfg.Cwd,
			HookEventName: hooks.EventPostToolUse,
			ToolName:      pending.ToolName,
			ToolUseID:     pending.ToolUseID,
			ToolInput:     pending.ToolInput,
			ToolResponse:  toolResponse(res),
			DurationMs:    &took,
		})
	}
}
