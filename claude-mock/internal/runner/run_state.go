package runner

import (
	"encoding/json"
	"sync"
)

// runState is what the run has done since its last result frame, from which the
// next result's fields are derived: the turns the main agent's model took, the
// tool calls a hook refused, and how many results came before.
type runState struct {
	mu             sync.Mutex
	turns          int
	denials        []map[string]any
	results        int
	stopErrorShown bool           // the notice of a Stop hook's error is shown once per run
	origin         map[string]any // what started the turn the next result ends, when not the user's prompt
	// local is the slash command the run was (its result says so), "" for a prompt for the model.
	local string
}

// startedBy records that the turn now running was started by something other than the user's prompt
// (a task's notification): the result that ends it names it as its origin (recorded: runs/bgagent).
func (r *runState) startedBy(origin map[string]any) {
	r.mu.Lock()
	r.origin = origin
	r.mu.Unlock()
}

func (r *runState) takeOrigin() map[string]any {
	r.mu.Lock()
	defer r.mu.Unlock()
	o := r.origin
	r.origin = nil
	return o
}

// runLocal notes that the run is a slash command the harness carried out itself.
func (r *runState) runLocal(command string) {
	r.mu.Lock()
	r.local = command
	r.mu.Unlock()
}

// isLocal is whether the run is a slash command of the harness's own.
func (r *runState) isLocal() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.local != ""
}

// takeLocal is the slash command the result is of, and forgets it.
func (r *runState) takeLocal() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	local := r.local
	r.local = ""
	return local
}

// turn counts a turn the model took: it called a tool or answered.
func (r *runState) turn() {
	r.mu.Lock()
	r.turns++
	r.mu.Unlock()
}

// addTurns counts several turns the model took at once.
func (r *runState) addTurns(n int) {
	r.mu.Lock()
	r.turns += n
	r.mu.Unlock()
}

// deny records a tool call a PreToolUse hook refused (the result's permission_denials).
func (r *runState) deny(call pendingToolUse) {
	var input any
	_ = json.Unmarshal(call.ToolInput, &input)
	r.mu.Lock()
	r.denials = append(r.denials, map[string]any{"tool_name": call.ToolName, "tool_use_id": call.ToolUseID, "tool_input": input})
	r.mu.Unlock()
}

// restore puts back what take returned, when no result was written after all.
func (r *runState) restore(turns int, denials []map[string]any, index int) {
	r.mu.Lock()
	r.turns, r.denials, r.results = turns, denials, index
	r.mu.Unlock()
}

// take is the state for the result being written, and starts the next
// result's count from nothing.
func (r *runState) take() (turns int, denials []map[string]any, index int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	turns, denials, index = r.turns, r.denials, r.results
	r.turns, r.denials, r.results = 0, nil, r.results+1
	return
}
