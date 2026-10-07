package runner

import (
	"context"
	"encoding/json"

	"github.com/sloprail/harness-mocks/claude-mock/internal/hooks"
)

// writeCall streams and records a tool_use the script made, ahead of its hooks (the tool_use is part of
// the trajectory whatever a hook decides), checking first that the mock implements the call
// (adr/tool-calls-validated). It returns the call as pending, its hooks not yet run.
func writeCall(cfg Config, tr *transcript, line []byte, id, name string, input json.RawMessage) (pendingToolUse, error) {
	if _, err := Schema().Check(name, input); err != nil {
		return pendingToolUse{}, err
	}
	line, streamed, input := withToolDefaults(cfg, withoutMore(line))
	writeToolUse(cfg, withoutMockResults(streamed), name, input) // the call keeps its mock_result for the tool; the frames do not show it
	tr.persist(withoutMockResults(line))
	call := pendingToolUse{ToolUseID: id, ToolName: name, ToolInput: input}
	if res := invalidCall(name, input, cfg.Cwd); res != nil {
		call.Invalid = res
	}
	return call, nil
}

// preTool fires the PreToolUse hooks of a call and records what they decided on it; a call that cannot be
// acted on has no hook. The call counts as started once its hooks have run: what another agent's gate may
// wait for.
func preTool(ctx context.Context, cfg Config, inv *hooks.Invoker, call *pendingToolUse) error {
	defer cfg.steps.started()
	if call.Invalid != nil {
		return nil
	}
	pre := hooks.Input{
		SessionID: cfg.SessionID, AgentID: cfg.AgentID, Cwd: cfg.Cwd, HookEventName: hooks.EventPreToolUse,
		ToolName: call.ToolName, ToolUseID: call.ToolUseID, ToolInput: hookInput(true, call.ToolName, call.ToolInput),
	}
	hookOut, runs, hookErr := inv.FireRuns(ctx, pre)
	writeHookEventFrames(cfg, pre, runs)
	if err := decidePreTool(cfg, call, hookOut, hookErr); err != nil {
		return err
	}
	answerFromHook(call, hookOut)
	return nil
}

// answerFromHook puts the answers a PreToolUse hook gave an AskUserQuestion (allow, with an updatedInput that
// holds the questions and their answers: the way a run with no terminal has them answered) into the call: the
// tool runs on that input, and PostToolUse sees it; the transcript keeps the call as the model made it
// (recorded: snapshots/runs/ask-user-question-tool).
// sr:docs https://code.claude.com/docs/en/hooks#allow-with-updatedinput
func answerFromHook(call *pendingToolUse, out hooks.Output) {
	h := out.HookSpecificOutput
	if call.ToolName != "AskUserQuestion" || call.Blocked || h == nil || h.PermissionDecision != "allow" || len(h.UpdatedInput) == 0 {
		return
	}
	call.ToolInput = h.UpdatedInput
}

// withoutMore is the line without the scenario's "more" marker on its tool_use block: it is no part of
// what the model sent.
func withoutMore(line []byte) []byte {
	if !toolUseMore(line) {
		return line
	}
	var m map[string]any
	if json.Unmarshal(line, &m) != nil {
		return line
	}
	msg, _ := m["message"].(map[string]any)
	blocks, _ := msg["content"].([]any)
	for _, b := range blocks {
		if block, _ := b.(map[string]any); block["type"] == "tool_use" {
			delete(block, "more")
		}
	}
	if out, err := marshalRecord(m); err == nil {
		return out
	}
	return line
}
