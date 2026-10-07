package toolexec

import (
	"context"
	"encoding/json"

	"github.com/sloprail/harness-mocks/internal/tools"
)

// skillInput is the argument shape for the Skill tool.
// sr:docs https://code.claude.com/docs/en/tools-reference#tool-behavior
type skillInput struct {
	Skill string `json:"skill"`
	Args  string `json:"args,omitempty"`
}

// reinvocation is the notice that comes ahead of a skill's instructions when the agent has launched the
// same skill before (recorded: snapshots/runs/skill-tool).
func reinvocation(name string) string {
	return "(Re-invocation of /" + name + " — the skill instructions were previously loaded; the arguments or dynamic output below are new.)"
}

// unknownSkill answers a call that names a skill that is not there, before any hook sees it (recorded:
// snapshots/runs/skill-tool: neither PreToolUse nor PostToolUse fired). ok is false for any other call.
func unknownSkill(toolName string, raw json.RawMessage, cwd string) (Result, bool) {
	var inp skillInput
	if toolName != "Skill" || json.Unmarshal(raw, &inp) != nil || inp.Skill == "" {
		return Result{}, false
	}
	if _, err := tools.FindSkill(cwd, inp.Skill); err == nil {
		return Result{}, false
	}
	msg := "Unknown skill: " + inp.Skill
	return Result{Output: "<tool_use_error>" + msg + "</tool_use_error>", IsError: true, ToolUseResult: "Error: " + msg}, true
}

// executeSkill launches a skill the way claude 2.1.285 does (recorded: snapshots/runs/skill-tool): the call's
// result says the skill is launching, and the skill's instructions (with the call's arguments) reach the agent
// as a message of their own, after the result, headed by a notice when the agent launched the skill before.
//
// sr:provides skill-tool/claude
func executeSkill(ctx context.Context, raw json.RawMessage, cwd, sessionID string) Result {
	var inp skillInput
	if err := json.Unmarshal(raw, &inp); err != nil || inp.Skill == "" {
		return Result{Output: "Skill: missing or invalid 'skill' field", IsError: true}
	}
	skill, err := tools.FindSkill(cwd, inp.Skill)
	if err != nil {
		res, _ := unknownSkill("Skill", raw, cwd)
		return res
	}
	var meta []MetaMessage
	if isKnown(ctx, sessionID, "skill:"+skill.Name) {
		meta = append(meta, MetaMessage{Text: reinvocation(skill.Name), Plain: true})
	}
	setKnown(ctx, sessionID, "skill:"+skill.Name, true)
	return Result{
		Output:        "Launching skill: " + skill.Name,
		ToolUseResult: map[string]any{"success": true, "commandName": skill.Name},
		Meta:          append(meta, MetaMessage{Text: skill.Instructions(inp.Args)}),
	}
}
