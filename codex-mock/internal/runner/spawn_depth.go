package runner

import (
	"sync"
	"time"

	"github.com/sloprail/harness-mocks/codex-mock/internal/session"
	"github.com/sloprail/harness-mocks/internal/subagents"
)

// DefaultAgentDepth is how many layers of sub-agents Codex lets a session nest
// when agents.max_depth is not set: one, so a sub-agent cannot spawn (recorded:
// runs/nested-subagents-limit, run with max_depth 2).
const DefaultAgentDepth = 1

// AgentSettings is what the entrypoint read of the command line and the
// environment for sub-agents.
type AgentSettings struct {
	// MaxDepth is agents.max_depth, zero when not set.
	MaxDepth int
	// Script is the scenario script a sub-agent runs when its spawn_agent call
	// does not carry one (A10N_MOCK_SUBAGENT_SCRIPT).
	Script string
}

// Agents is set once by the entrypoint, before Run.
var Agents AgentSettings

// agentPos is where a thread sits: the main thread (ID "", depth 0) or a
// sub-agent.
type agentPos struct {
	ID    string
	Depth int
}

// threads is the position of every sub-agent started, by thread id (the main
// thread is in none). One run per process, so one table.
var threads sync.Map

func posOf(thread string) agentPos {
	p, _ := threads.Load(thread)
	pos, _ := p.(agentPos)
	return pos
}

// canDispatch is whether the agent at pos is offered the tool that spawns
// sub-agents: one at the depth limit is not (recorded: its tool search finds
// nothing), so a call of it there is the unknown tool every other is.
func canDispatch(pos agentPos) bool {
	limit := Agents.MaxDepth
	if limit <= 0 {
		limit = DefaultAgentDepth
	}
	return subagents.Parent{ID: pos.ID, Depth: pos.Depth}.CanDispatch(limit)
}

// byAgent adds to a tool hook's own fields the agent making the call, when it
// is a sub-agent: agent_id and agent_type, for every tool it uses; the main
// thread's calls name none (recorded: runs/nested-subagents).
func (h toolHost) byAgent(own map[string]any) map[string]any {
	if pos := posOf(h.id); pos.ID != "" {
		own["agent_id"], own["agent_type"] = pos.ID, agentType
	}
	return own
}

// createSub starts the rollout of the sub-agent subID the calling agent spawns,
// one layer deeper than it (the core's placement): the thread's meta record
// names its parent thread and depth, and the agent is known from now on at that
// depth, to the tools it is offered and the hooks of its calls. All of a
// session's rollouts sit in one directory.
// sr:provides nested-subagents/codex
func (h toolHost) createSub(subID string) (*session.File, error) {
	pos := posOf(h.id)
	parent := pos.ID
	if parent == "" {
		parent = h.hooks.Common.SessionID
	}
	place := subagents.Place(subagents.Parent{ID: parent, Depth: pos.Depth})
	rollout, err := session.CreateSub(h.cfg.CodexHome, subID, h.cfg.Cwd, time.Now(),
		session.Origin{Session: h.hooks.Common.SessionID, Parent: place.ParentID, Depth: place.Depth})
	if err == nil {
		threads.Store(subID, agentPos{ID: subID, Depth: place.Depth})
	}
	return rollout, err
}
