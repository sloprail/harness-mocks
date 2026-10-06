package replay

import (
	"fmt"
	"os"
	"path/filepath"

	core "github.com/sloprail/harness-mocks/internal/replay"
)

// stepThreads are the session each run worked in, by the id the recording gave it.
func stepThreads(specs []stepSpec, first string) ([]string, error) {
	threads := []string{first}
	for i, s := range specs[1:] {
		switch {
		case s.resume != "" && s.resume != first:
			return nil, unbuildable(fmt.Errorf("a later run resumes %s, a session the recording does not hold", s.resume))
		case s.resume != "" && s.fork && s.newID != "": // a fork carries the session on under an id of its own
			threads = append(threads, s.newID)
		case s.resume != "" && s.fork:
			return nil, unbuildable(fmt.Errorf("a later run forks a session under an id the recording does not name"))
		case s.resume != "":
			threads = append(threads, first)
		case s.cont && threads[i] != first:
			return nil, unbuildable(fmt.Errorf("a later run continues a session other than the first"))
		case s.cont:
			threads = append(threads, first)
		case s.newID != "":
			threads = append(threads, s.newID)
		default:
			return nil, unbuildable(fmt.Errorf("a later run starts no session of a known id"))
		}
	}
	return threads, nil
}

// agentsOf are what the model did in each run of claude the recording holds: the earlier runs the
// preparation made, the first run's agent, and each later one as a core.Step, with the sub-agents
// they started attached. A run alone is the whole transcript of its session; runs share a
// session's transcript, so each is what follows its prompt there. A session that left no
// transcript (--no-session-persistence) is read from the stream, when it holds the main thread's
// turns only.
func agentsOf(setup, sample string, stream []map[string]any) (core.Agent, []core.Step, []earlierRun, error) {
	specs, err := stepSpecs(setup)
	if err != nil {
		return core.Agent{}, nil, nil, err
	}
	pre, err := earlierSpecs(readFile(filepath.Join(setup, "prepare.sh")))
	if err != nil {
		return core.Agent{}, nil, nil, err
	}
	session := sessionOf(stream)
	if session == "" {
		return core.Agent{}, nil, nil, unbuildable(fmt.Errorf("the stream names no session"))
	}
	threads := []string{session}
	if len(specs) > 1 {
		if threads, err = stepThreads(specs, session); err != nil {
			return core.Agent{}, nil, nil, err
		}
	}
	all, allThreads := append(append([]stepSpec{}, pre...), specs...), []string{}
	for _, p := range pre {
		allThreads = append(allThreads, p.newID)
	}
	allThreads = append(allThreads, threads...)
	sessions, subs := map[string][]map[string]any{}, map[string]turns{}
	for _, th := range allThreads {
		if _, done := sessions[th]; done {
			continue
		}
		path := filepath.Join(sample, "transcript", th+".jsonl")
		if _, err := os.Stat(path); err != nil && th == session {
			recs, err := mainThread(stream)
			if err != nil {
				return core.Agent{}, nil, nil, err
			}
			sessions[th] = recs
			continue
		}
		recs, err := readJSONL(path)
		if err != nil {
			return core.Agent{}, nil, nil, err
		}
		if len(recs) == 0 {
			return core.Agent{}, nil, nil, unbuildable(fmt.Errorf("no transcript of the session %s was recorded: the model's turns are unknown", th))
		}
		sessions[th] = recs
		more, err := subagentTurns(filepath.Join(sample, "transcript", th, "subagents"))
		if err != nil {
			return core.Agent{}, nil, nil, err
		}
		for id, t := range more {
			subs[id] = t
		}
	}
	wire := wireInputs(stream)
	records := [][]map[string]any{sessions[session]}
	if len(all) > 1 {
		if records, err = stepRecords(all, allThreads, sessions); err != nil {
			return core.Agent{}, nil, nil, err
		}
	}
	var agents []core.Agent
	for _, recs := range records {
		t, err := modelTurns(recs)
		if err != nil {
			return core.Agent{}, nil, nil, unbuildable(err)
		}
		agents = append(agents, attachSubagents(withWireInputs(t, wire), subs))
	}
	if len(subs) > 0 {
		return core.Agent{}, nil, nil, unbuildable(fmt.Errorf("a sub-agent whose starting call is in no transcript"))
	}
	var earlier []earlierRun
	for i, p := range pre {
		earlier = append(earlier, earlierRun{Prompt: p.prompt, ID: p.newID, Agent: agents[i]})
	}
	agents = agents[len(pre):]
	var then []core.Step
	for i, s := range specs[1:] {
		then = append(then, core.Step{Prompt: s.prompt, Args: stepMockArgs(s), Cwd: s.cwd, Agent: agents[i+1]})
	}
	return agents[0], then, earlier, nil
}

// mainThread are the records of the main thread's turns that the stream holds: the assistant frames
// it wrote, as a transcript holds them. A stream that holds a sub-agent's frames too is not read this
// way.
func mainThread(stream []map[string]any) ([]map[string]any, error) {
	var recs []map[string]any
	for _, f := range stream {
		if f["type"] != "assistant" {
			continue
		}
		if f["parent_tool_use_id"] != nil {
			return nil, unbuildable(fmt.Errorf("no transcript of the main session was recorded, and the stream holds a sub-agent's turns: the model's turns are unknown"))
		}
		recs = append(recs, f)
	}
	return recs, nil
}
