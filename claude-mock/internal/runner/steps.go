package runner

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/sloprail/harness-mocks/internal/scenario"
	"github.com/sloprail/harness-mocks/internal/subagents"
)

// agentSteps is what a script's gates are read against for one agent: how far it has got through its
// calls, the sub-agents it has started, and how far the agent that started it has got. Its methods
// accept nil, which has done nothing and waits for nothing.
type agentSteps struct {
	prog    *subagents.Progress // this agent's calls started and finished
	spawned *subagents.SpawnLog // the sub-agents this agent started, in order
	parent  *subagents.Progress // the progress of the agent that started this one (nil: the session's own)
}

// newAgentSteps are the steps of an agent started by the agent whose steps are given (nil: the session's own).
func newAgentSteps(parent *agentSteps) *agentSteps {
	s := &agentSteps{prog: subagents.NewProgress(), spawned: &subagents.SpawnLog{}}
	if parent != nil {
		s.parent = parent.prog
	}
	return s
}

// started counts a call the agent has begun, finished one it has given the result of.
func (s *agentSteps) started() {
	if s != nil {
		s.prog.Move(1, 0)
	}
}

func (s *agentSteps) finished() {
	if s != nil {
		s.prog.Move(0, 1)
	}
}

// spawn records a sub-agent the agent has started; a foreground one has ended when its call returns.
func (s *agentSteps) spawn(id string, foreground bool) {
	if s == nil {
		return
	}
	if foreground {
		s.spawned.AddSettled(id)
		return
	}
	s.spawned.Add(id)
}

// hold holds the agent's next step back until what the script's gate names has happened; a gate that
// names what does not exist is reported, in the mock's words.
func (s *agentSteps) hold(ctx context.Context, cfg Config, g *scenario.Gate) {
	if s == nil || g == nil || g.None() {
		return
	}
	for _, p := range subagents.Hold(ctx, *g, cfg.bg.Registry, s.spawned, s.parent) {
		fmt.Fprintf(cfg.Stderr, "ERROR claude_mock: %s\n", p)
	}
}

// writeToolUse streams a tool call's frame and, for a sub-agent, its task_progress frame: ahead of the
// call's for a foreground one, after it for a background one (recorded: runs/fg-subagent-bash, bgagent).
func writeToolUse(cfg Config, frame []byte, tool string, input json.RawMessage) {
	if cfg.AgentID != "" && !cfg.background {
		cfg.progress(cfg, tool, input)
	}
	writeStreamLine(cfg, frame)
	if cfg.AgentID != "" && cfg.background {
		cfg.progress(cfg, tool, input)
	}
}
