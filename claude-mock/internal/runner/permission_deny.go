package runner

import (
	"context"

	"github.com/sloprail/harness-mocks/claude-mock/internal/hooks"
	"github.com/sloprail/harness-mocks/claude-mock/internal/toolexec"
)

// ruleDenial is the refusal of a tool call by a deny rule of the settings: the PreToolUse hooks
// have run on it (they saw the call), the tool does not, and no PostToolUse fires. The agent is
// told "Permission to use Bash with command <command> has been denied.", the stream carries a
// permission_denied system frame ahead of that result, and the run's result lists the call in
// permission_denials (recorded: snapshots/runs/permission-denied).
// sr:docs https://code.claude.com/docs/en/headless#auto-approve-tools
func ruleDenial(inv *hooks.Invoker, call pendingToolUse) (text string, denied bool) {
	command, denied := inv.Denied(call.ToolName, call.ToolInput)
	if !denied {
		return "", false
	}
	return "Permission to use " + call.ToolName + " with command " + command + " has been denied.", true
}

// denyByRule answers a call a deny rule refuses, and goes on to the next turn.
func denyByRule(ctx context.Context, cfg Config, inv *hooks.Invoker, tr *transcript, bg *backgroundTasks, call pendingToolUse, text, lastText string) (turnResult, error) {
	writeFrame(cfg, map[string]any{
		"type": "system", "subtype": "permission_denied", "tool_name": call.ToolName, "tool_use_id": call.ToolUseID,
		"decision_reason_type": "rule", "message": text,
	})
	res := toolexec.Result{Output: text, IsError: true, ToolUseResult: "Error: " + text, NonExecution: "permission-rule"}
	if err := emitToolResult(cfg, call, res, tr); err != nil {
		return turnResult{}, err
	}
	bg.run.deny(call)
	bg.deliverMidTurn(ctx, cfg, inv, tr)
	return turnResult{sig: "denied:" + call.ToolName + ":" + string(call.ToolInput), lastText: lastText}, nil
}
