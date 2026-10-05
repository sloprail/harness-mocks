package runner

import (
	"context"
	"encoding/json"
	"io"
	"strings"

	"github.com/sloprail/harness-mocks/cursor-mock/internal/hooks"
	"github.com/sloprail/harness-mocks/internal/scenario"
	coresession "github.com/sloprail/harness-mocks/internal/session"
	"github.com/sloprail/harness-mocks/internal/tasks"
	"github.com/sloprail/harness-mocks/internal/turnloop"
)

// startsBackgroundSubagent reports whether the call is a Task the mock runs as a
// sub-agent in the background: one with its description, prompt and the script
// that plays the sub-agent, asking to run in the background.
func startsBackgroundSubagent(tu scenario.ToolUse) (taskInput, bool) {
	var in taskInput
	if tu.Name != "Task" || json.Unmarshal(tu.Input, &in) != nil || in.RunInBackground == nil || !*in.RunInBackground {
		return in, false
	}
	return in, in.Description != "" && in.Prompt != "" && in.Script != ""
}

// launchSubagent is a background Task call: the parent's preToolUse hook, the
// call on the stream, answered at once with the sub-agent's id (isBackground),
// and the sub-agent running in a conversation of its own while the parent's
// turn goes on, registered with the session's background tasks. A session
// does not end while it runs (afterTurn); its end is announced and starts a
// further turn. The call fires no postToolUse, and no hook of it can refuse it
// here (recorded: runs/print-waits-for-background-agents; adr/modeled-surface).
//
// sr:provides background-agent/cursor
func (s *session) launchSubagent(ctx context.Context, tu scenario.ToolUse, in taskInput) {
	typ, args := s.announceTask(ctx, tu, in)
	sub := *s
	sub.id, sub.parent = coresession.NewID(), s
	sub.owner = sub.id
	sub.cfg.Stdout, sub.cfg.Script, sub.cfg.Prompt = io.Discard, in.Script, in.Prompt
	sub.texts, sub.added, sub.named = nil, nil, false
	var err error
	if sub.tr, err = newTranscript(s.cfg.Home, s.cfg.Dir, sub.id); err != nil {
		sub.tr = s.tr
	}
	sub.hooks = &hooks.Hooks{Config: s.hooks.Config, Dir: s.cfg.Dir, Env: sub.hookEnv, Common: sub.common}

	s.forward(taskFrame(s.id, tu.ID, "started", args, nil))
	s.tr.toolUse(tu.Name, map[string]any{"description": in.Description, "prompt": in.Prompt, "subagent_type": typ, "run_in_background": true})

	t := tasks.NewTask(tasks.Agent, sub.id)
	t.ToolUseID, t.Description, t.Owner, t.Meta = tu.ID, in.Description, s.owner, in.Prompt
	s.registry().StartAgent(t, func(ctx context.Context) {
		sub.tr.user(in.Prompt)
		if _, err := turnloop.Run(ctx, &sub, turnloop.Params{Script: in.Script, Dir: s.cfg.Dir, Environ: s.cfg.Environ, Prompt: in.Prompt, Added: sub.Context}); err != nil {
			t.Failure = err.Error()
		}
		sub.named = true
		sub.tr.end()
		t.Result = strings.Join(sub.texts, "")
	})
	s.forward(taskFrame(s.id, tu.ID, "completed", args, map[string]any{"success": map[string]any{
		"conversationSteps": []any{}, "agentId": sub.id, "isBackground": true, "durationMs": "0",
		"backgroundReason": "SUBAGENT_BACKGROUND_REASON_AGENT_REQUEST"}}))
}
