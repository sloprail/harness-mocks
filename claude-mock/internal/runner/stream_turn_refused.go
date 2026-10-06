package runner

import (
	"context"

	"github.com/sloprail/harness-mocks/claude-mock/internal/hooks"
	"github.com/sloprail/harness-mocks/claude-mock/internal/toolexec"
)

// answerRefusedCall answers a call that does not run: one the tool cannot take, or one a PreToolUse hook
// refused. refused is false for a call that goes on to run.
func answerRefusedCall(ctx context.Context, cfg Config, inv *hooks.Invoker, tr *transcript, bg *backgroundTasks, sc scanResult) (res turnResult, refused bool, err error) {
	pending := sc.pending
	// Input the tool cannot take: its tool_use_error is the result, and the
	// turn goes on; no hook fires (recorded: snapshots/runs/tool-invalid-input).
	if pending.Invalid != nil {
		if err := emitToolResult(cfg, pending, *pending.Invalid, tr); err != nil {
			return turnResult{}, true, err
		}
		return turnResult{sig: "invalid:" + pending.ToolName + ":" + string(pending.ToolInput), lastText: sc.lastText}, true, nil
	}

	// PreToolUse REFUSED this tool call — an exit-0 permissionDecision deny, or an exit 2. The
	// tool does not run and no PostToolUse fires; the refusal is the tool_result, "PreToolUse:<Tool>
	// hook error: <reason>" (an exit 2's reason is "[<command>]: <stderr>"), and the turn goes on
	// (claude 2.1.282). The loop guard signature is the blocked tool_use, so a re-emitted call is bounded.
	// sr:docs https://code.claude.com/docs/en/hooks#pretooluse
	if pending.Blocked {
		text := "PreToolUse:" + pending.ToolName + " hook error: " + pending.BlockReason
		blockRes := toolexec.Result{Output: text, IsError: true, ToolUseResult: "Error: " + text, NonExecution: "permission-rule"}
		if err := emitToolResult(cfg, pending, blockRes, tr); err != nil {
			return turnResult{}, true, err
		}
		bg.run.deny(pending)
		bg.deliverMidTurn(ctx, cfg, inv, tr)
		return turnResult{sig: "blocked:" + pending.ToolName + ":" + string(pending.ToolInput), lastText: sc.lastText}, true, nil
	}
	return turnResult{}, false, nil
}
