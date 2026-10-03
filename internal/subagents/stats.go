package subagents

import "sync"

// Part of the background-agent capability (its core marker is on
// tasks.Registry.StartAgent): the sub-agents of a session, counted.
//
// Ask is what a call asked for: the foreground, the background, or neither.
type Ask int

const (
	Unset Ask = iota
	AskedBackground
	AskedForeground
)

// Count is a session's count of its sub-agents.
type Count struct {
	Spawned, StartedInBackground, MaxDepth, SpawnedBySubagents int
	Requested                                                  map[Ask]int
	Completed, Failed, RefusedConcurrency                      int
	ByType                                                     map[string]int
}

// Stats is a session's running count of its sub-agents, safe for the root run
// and the sub-agents it runs concurrently to share.
type Stats struct {
	mu sync.Mutex
	t  Count
}

// Spawn counts a sub-agent started: what its call asked for, whether it began in
// the background, how deep it is and whether another sub-agent spawned it.
func (s *Stats) Spawn(ask Ask, background bool, depth int, bySubagent bool, agentType string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.t.Requested == nil {
		s.t.Requested, s.t.ByType = map[Ask]int{}, map[string]int{}
	}
	s.t.Spawned++
	s.t.Requested[ask]++
	s.t.ByType[agentType]++
	if background {
		s.t.StartedInBackground++
	}
	s.t.MaxDepth = max(s.t.MaxDepth, depth)
	if bySubagent {
		s.t.SpawnedBySubagents++
	}
}

// End counts a sub-agent that has ended, failed or not.
func (s *Stats) End(failed bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if failed {
		s.t.Failed++
	} else {
		s.t.Completed++
	}
}

// RefuseConcurrent counts a spawn refused for the concurrent limit.
func (s *Stats) RefuseConcurrent() {
	s.mu.Lock()
	s.t.RefusedConcurrency++
	s.mu.Unlock()
}

// Count is the count so far.
func (s *Stats) Count() Count {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := s.t
	t.Requested, t.ByType = map[Ask]int{}, map[string]int{}
	for k, v := range s.t.Requested {
		t.Requested[k] = v
	}
	for k, v := range s.t.ByType {
		t.ByType[k] = v
	}
	return t
}
