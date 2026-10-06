package runner

import (
	"context"

	"github.com/sloprail/harness-mocks/cursor-mock/internal/hooks"
	"github.com/sloprail/harness-mocks/internal/scenario"
	coresession "github.com/sloprail/harness-mocks/internal/session"
)

// announceTask is what a Task call does first, foreground or background: its
// preToolUse hooks are told the call as the model made it (the optional model
// and run_in_background only when it gave them; the type generalPurpose when it
// gave none), and the args of its stream frames are built. The hook's payload
// says no model and names the model request as its generation (recorded:
// runs/foreground-subagent-result, runs/subagent-worktree-isolation).
func (s *session) announceTask(ctx context.Context, tu scenario.ToolUse, in taskInput) (typ string, args map[string]any) {
	typ = in.SubagentType
	if typ == "" {
		typ = "generalPurpose"
	}
	input := map[string]any{"description": in.Description, "prompt": in.Prompt, "subagent_type": typ}
	if in.Model != nil {
		input["model"] = *in.Model
	}
	if in.RunInBackground != nil {
		input["run_in_background"] = *in.RunInBackground
	}
	useID := tu.ID
	if in.HookToolUseID != "" {
		useID = in.HookToolUseID
	}
	tool := hooks.Tool{Name: "Task", UseID: useID, Input: input}
	own := hooks.ToolFields(tool)
	own["model"], own["generation_id"] = "", s.requestID
	s.hooks.Fire(ctx, hooks.PreToolUse, tool.Name, own)
	s.named = true
	return typ, map[string]any{
		"description": in.Description, "prompt": in.Prompt, "subagentType": streamType(typ),
		"model": "default", "agentId": coresession.NewID(), "attachments": []any{}, "mode": "TASK_MODE_UNSPECIFIED",
		"respondingToMessageIds": []any{}, "environment": "SUBAGENT_EXECUTION_ENVIRONMENT_UNSPECIFIED",
		"machine": map[string]any{"sameMachine": map[string]any{}},
	}
}

// streamType is the sub-agent type as the stream's frames say it: the hook says
// generalPurpose where the stream says unspecified, a built-in type is its own
// name (shell), and any other is a custom agent named so (recorded:
// runs/foreground-subagent-result, runs/subagent-lifecycle-hooks,
// runs/subagent-worktree-isolation).
func streamType(typ string) map[string]any {
	switch typ {
	case "generalPurpose":
		return map[string]any{"unspecified": map[string]any{}}
	case "shell":
		return map[string]any{"shell": map[string]any{}}
	}
	return map[string]any{"custom": map[string]any{"name": typ}}
}
