package replay

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	core "github.com/sloprail/harness-mocks/internal/replay"
)

// stepSpecs are the runs of claude the setup holds, the run's own first. A later run's directory
// holds its prompt, its args and nothing else the adapter installs.
func stepSpecs(setup string) ([]stepSpec, error) {
	args, err := parseArgs(readFile(filepath.Join(setup, "args")))
	if err != nil {
		return nil, err
	}
	specs := []stepSpec{{prompt: strings.TrimSpace(readFile(filepath.Join(setup, "prompt.txt"))), args: args}}
	// a later run is a directory then/NN (prompt.txt, args), or the flat files then-NN-prompt.txt and then-NN-args
	var paths [][2]string // each run's prompt and args file
	dirs, _ := filepath.Glob(filepath.Join(setup, "then", "*"))
	for _, d := range dirs {
		entries, _ := os.ReadDir(d)
		for _, e := range entries {
			if e.Name() != "prompt.txt" && e.Name() != "args" {
				return nil, unbuildable(fmt.Errorf("a later run has %s, which the adapter does not install", e.Name()))
			}
		}
		paths = append(paths, [2]string{filepath.Join(d, "prompt.txt"), filepath.Join(d, "args")})
	}
	flat, _ := filepath.Glob(filepath.Join(setup, "then-*-prompt.txt"))
	for _, p := range flat {
		base := strings.TrimSuffix(p, "prompt.txt")
		paths = append(paths, [2]string{p, base + "args"})
	}
	sort.Slice(paths, func(i, j int) bool { return paths[i][0] < paths[j][0] })
	for _, p := range paths {
		s, err := parseStepArgs(readFile(p[1]))
		if err != nil {
			return nil, err
		}
		s.prompt = strings.TrimSpace(readFile(p[0]))
		specs = append(specs, s)
	}
	return specs, nil
}

// stepThreads are the session each run worked in, by the id the recording gave it.
func stepThreads(specs []stepSpec, first string) ([]string, error) {
	threads := []string{first}
	for i, s := range specs[1:] {
		switch {
		case s.resume != "" && s.resume != first:
			return nil, unbuildable(fmt.Errorf("a later run resumes %s, a session the recording does not hold", s.resume))
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

// agentsOf are what the model did in each run of claude the setup holds: the first run's agent,
// and each later one as a core.Step, with the sub-agents they started attached. A run alone is
// the whole transcript of its session; runs share a session's transcript, so each is what
// follows its prompt there.
func agentsOf(setup, sample string, stream []map[string]any) (core.Agent, []core.Step, error) {
	specs, err := stepSpecs(setup)
	if err != nil {
		return core.Agent{}, nil, err
	}
	session := sessionOf(stream)
	if session == "" {
		return core.Agent{}, nil, unbuildable(fmt.Errorf("the stream names no session"))
	}
	threads := []string{session}
	if len(specs) > 1 {
		if threads, err = stepThreads(specs, session); err != nil {
			return core.Agent{}, nil, err
		}
	}
	sessions, subs := map[string][]map[string]any{}, map[string]turns{}
	for _, th := range threads {
		if _, done := sessions[th]; done {
			continue
		}
		recs, err := readJSONL(filepath.Join(sample, "transcript", th+".jsonl"))
		if err != nil {
			return core.Agent{}, nil, err
		}
		if len(recs) == 0 {
			return core.Agent{}, nil, unbuildable(fmt.Errorf("no transcript of the main session was recorded: the model's turns are unknown"))
		}
		sessions[th] = recs
		more, err := subagentTurns(filepath.Join(sample, "transcript", th, "subagents"))
		if err != nil {
			return core.Agent{}, nil, err
		}
		for id, t := range more {
			subs[id] = t
		}
	}
	records := [][]map[string]any{sessions[session]}
	if len(specs) > 1 {
		if records, err = stepRecords(specs, threads, sessions); err != nil {
			return core.Agent{}, nil, err
		}
	}
	var agents []core.Agent
	for _, recs := range records {
		t, err := modelTurns(recs)
		if err != nil {
			return core.Agent{}, nil, unbuildable(err)
		}
		agents = append(agents, attachSubagents(t, subs))
	}
	if len(subs) > 0 {
		return core.Agent{}, nil, unbuildable(fmt.Errorf("a sub-agent whose starting call is in no transcript"))
	}
	var then []core.Step
	for i, s := range specs[1:] {
		then = append(then, core.Step{Prompt: s.prompt, Args: stepMockArgs(s), Agent: agents[i+1]})
	}
	return agents[0], then, nil
}

// stepMockArgs are the words a later run gives the mock: its session flags (the first session is
// the replay's own, <SESSION>) and the flags the mock models.
func stepMockArgs(s stepSpec) []string {
	var out []string
	switch {
	case s.resume != "":
		out = append(out, "--resume", "<SESSION>")
	case s.cont:
		out = append(out, "--continue")
	case s.newID != "":
		out = append(out, "--session-id", s.newID)
	}
	return append(out, s.args...)
}
