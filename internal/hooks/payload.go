package hooks

import (
	"encoding/json"
	"time"
)

// Agent names a sub-agent: its id and its type.
type Agent struct{ ID, Type string }

// Common is what a hook's payload carries beside the event's own fields: the
// transcript file, the working directory and, inside a sub-agent, that
// sub-agent.
type Common struct {
	TranscriptPath string
	Cwd            string
	Agent          Agent
}

// CommonFields completes an event's common fields from the session's: the
// transcript file and the working directory default to the session's, and an
// event raised inside a sub-agent (inside.ID is set) names that sub-agent,
// with its type, unless the event already names an agent of its own. An event
// on the main thread (inside empty) names none.
//
// sr:capability hook-common-payload
func CommonFields(event, session Common, inside Agent) Common {
	c := event
	if c.TranscriptPath == "" {
		c.TranscriptPath = session.TranscriptPath
	}
	if c.Cwd == "" {
		c.Cwd = session.Cwd
	}
	if inside.ID == "" {
		return c
	}
	if c.Agent.ID == "" {
		c.Agent.ID = inside.ID
	}
	if c.Agent.ID == inside.ID && c.Agent.Type == "" {
		c.Agent.Type = inside.Type
	}
	return c
}

// PostTool is what a hook after a successful tool call is told: the call's
// input, its structured response and how long the call took.
type PostTool struct {
	Input      json.RawMessage
	Response   json.RawMessage
	DurationMs int64
}

// NewPostTool is the notice for a tool call that succeeded. The duration is
// the call's own run time, in whole milliseconds and never negative.
//
// sr:capability posttooluse-payload
func NewPostTool(input, response json.RawMessage, took time.Duration) PostTool {
	ms := took.Milliseconds()
	if ms < 0 {
		ms = 0
	}
	return PostTool{Input: input, Response: response, DurationMs: ms}
}

// Stop is what a hook after the agent finishes responding is told: the
// agent's last message, and whether the agent is already continuing because
// an earlier hook blocked.
type Stop struct {
	LastMessage string
	Continuing  bool
}

// NewStop is the notice for the agent finishing a response, after blocksSoFar
// consecutive blocks by earlier stop hooks.
//
// sr:capability stop-hook-payload
func NewStop(lastMessage string, blocksSoFar int) Stop {
	return Stop{LastMessage: lastMessage, Continuing: blocksSoFar > 0}
}
