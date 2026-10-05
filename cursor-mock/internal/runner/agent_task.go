package runner

import (
	"context"
	"encoding/json"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/sloprail/harness-mocks/cursor-mock/internal/hooks"
	"github.com/sloprail/harness-mocks/internal/scenario"
	coresession "github.com/sloprail/harness-mocks/internal/session"
	"github.com/sloprail/harness-mocks/internal/tasks"
	"github.com/sloprail/harness-mocks/internal/turnloop"
)

// taskInput is the input of a Task call as the mock takes it: what Cursor's
// carries (description, prompt, subagent_type) and, mock-only, the scenario
// script that plays the sub-agent (the mock cannot let a model decide).
type taskInput struct {
	Description  string `json:"description"`
	Prompt       string `json:"prompt"`
	SubagentType string `json:"subagent_type"`
	Script       string `json:"script"`
}

// dispatchesSubagent reports whether the call is a foreground Task the mock
// runs as a sub-agent: one with its description, prompt and the script that
// plays the sub-agent. A background Task, and a main agent's call that lacks
// them, are not modelled (adr/modeled-surface); they end as a call to a tool
// the mock does not have.
func dispatchesSubagent(tu scenario.ToolUse) (taskInput, bool) {
	var in taskInput
	if tu.Name != "Task" || json.Unmarshal(tu.Input, &in) != nil {
		return in, false
	}
	return in, in.Description != "" && in.Prompt != "" && in.Script != ""
}

// startSubagent is the first half of a foreground Task call: the parent's
// preToolUse hooks and the call on the stream; it returns the second half.
func (s *session) startSubagent(ctx context.Context, tu scenario.ToolUse, in taskInput) (finish func()) {
	typ := in.SubagentType
	if typ == "" {
		typ = "generalPurpose"
	}
	tool := hooks.Tool{Name: "Task", UseID: tu.ID, Input: map[string]any{
		"description": in.Description, "prompt": in.Prompt, "subagent_type": typ, "run_in_background": false}}
	s.hooks.Fire(ctx, hooks.PreToolUse, tool.Name, hooks.ToolFields(tool))
	s.named = true

	args := map[string]any{
		"description": in.Description, "prompt": in.Prompt, "subagentType": map[string]any{typ: map[string]any{}},
		"model": "default", "agentId": coresession.NewID(),
	}
	s.forward(taskFrame(s.id, tu.ID, "started", args, nil))
	s.tr.toolUse(tu.Name, map[string]any{"description": in.Description, "prompt": in.Prompt, "subagent_type": typ})
	return func() { s.finishSubagent(ctx, tu, in, typ, args) }
}

// finishSubagent is the second half of a foreground Task call (the first is
// startSubagent: the parent's preToolUse hooks and the call on the stream): the
// sub-agent running to its final response in a conversation of its own (its own session id and transcript, its tool calls
// fired to the hooks under that id and not shown on the stream), and the call
// completed with what it said. A command the sub-agent started in the
// background is terminated when it gives its final response (recorded:
// runs/foreground-subagent-bash-ends-with-response); no hook of the Task call
// can refuse it here, and subagentStart and subagentStop were not recorded
// firing in a print-mode run (adr/modeled-surface).
//
// The call's result is the sub-agent's report: its conversation steps (its
// final response the last), its agent id, that it did not run in the background
// and how long it took (recorded: runs/foreground-subagent-result).
//
// sr:provides foreground-subagent-bash-ends-with-response/cursor
// sr:provides foreground-subagent-result/cursor
// sr:docs https://cursor.com/docs/hooks#subagentstop
func (s *session) finishSubagent(ctx context.Context, tu scenario.ToolUse, in taskInput, typ string, args map[string]any) {
	started := time.Now()
	sub := *s
	sub.id, sub.parent = coresession.NewID(), s
	sub.owner = sub.id
	sub.cfg.Stdout, sub.cfg.Script, sub.cfg.Prompt = io.Discard, in.Script, in.Prompt
	sub.texts, sub.added, sub.named = nil, nil, false
	var err error
	if sub.tr, err = newSubagentTranscript(s.tr, sub.id); err != nil {
		sub.tr = s.tr
	}
	sub.hooks = &hooks.Hooks{Config: s.hooks.Config, Dir: s.cfg.Dir, Env: sub.hookEnv, Common: sub.common}
	sub.tr.user(in.Prompt)
	_, _ = turnloop.Run(ctx, &sub, turnloop.Params{Script: in.Script, Dir: s.cfg.Dir, Environ: s.cfg.Environ, Prompt: in.Prompt, Added: sub.Context})
	sub.named = true
	sub.tr.end()
	result := map[string]any{"success": map[string]any{
		"conversationSteps": []any{map[string]any{"assistantMessage": map[string]any{"text": strings.Join(sub.texts, "")}}},
		"agentId":           sub.id, "isBackground": false, "durationMs": strconv.FormatInt(time.Since(started).Milliseconds(), 10),
		"backgroundReason": "SUBAGENT_BACKGROUND_REASON_UNSPECIFIED",
	}}
	s.forward(taskFrame(s.id, tu.ID, "completed", args, result))
	// the sub-agent has given its final response: what it left running ends, before
	// the parent goes on
	var ending []*tasks.Task
	for _, t := range s.registry().Running() {
		if t.Owner == sub.owner {
			ending = append(ending, t)
		}
	}
	s.registry().EndOfResponse(sub.owner)
	for _, t := range ending {
		s.forward(notificationFrame(s.id, t))
	}
}

// taskFrame is a Task call's tool_call frame.
func taskFrame(session, id, subtype string, args, result map[string]any) []byte {
	body := map[string]any{"args": args}
	if result != nil {
		body["result"] = result
	}
	return jsonLine(map[string]any{
		"type": "tool_call", "subtype": subtype, "call_id": id, "session_id": session,
		"tool_call": map[string]any{"taskToolCall": body},
	})
}
