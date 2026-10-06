package runner

import (
	"context"

	"github.com/sloprail/harness-mocks/claude-mock/internal/hooks"
)

// postScenarioResult fires PostToolUse for a tool_result the scenario wrote itself (a tool the
// mock does not run, e.g. an AskUserQuestion answer). Real PostToolUse names the call by its
// tool_use_id, also its attachment's toolUseID, and the tool by the tool_use it answers.
// sr:docs https://docs.anthropic.com/en/docs/claude-code/hooks#posttooluse
func postScenarioResult(ctx context.Context, cfg Config, inv *hooks.Invoker, tr *transcript, line []byte) error {
	toolUseID, toolName, toolOutput := extractFirstToolResult(line)
	if toolName == "" && toolUseID != "" {
		toolName = toolNameInTranscript(tr, toolUseID)
	}
	if toolName == "" {
		return nil
	}
	if err := refuseUnrecordedHook(cfg, inv, hooks.EventPostToolUse); err != nil {
		return err // a scenario-written tool_result
	}
	_, _ = inv.Fire(ctx, hooks.Input{
		SessionID:     cfg.SessionID,
		Cwd:           cfg.Cwd,
		HookEventName: hooks.EventPostToolUse,
		ToolName:      toolName,
		ToolUseID:     toolUseID,
		ToolResponse:  toolOutput,
	})
	return nil
}
