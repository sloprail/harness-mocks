package runner

import "sync"

// agentRegistry holds the session's sub-agents by id: a message to one resumes it.
type agentRegistry struct {
	mu sync.Mutex
	by map[string]*subagentRun
}

func (r *agentRegistry) remember(s *subagentRun) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.by == nil {
		r.by = map[string]*subagentRun{}
	}
	r.by[s.agentID] = s
}

func (r *agentRegistry) find(id string) *subagentRun {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.by[id]
}
