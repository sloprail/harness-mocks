package replay

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	core "github.com/sloprail/harness-mocks/internal/replay"
)

// stepFile is a setup file of a later run (capture.sh: then-<NN>-prompt.txt, then-<NN>-args, then-<NN>-cwd).
var stepFile = regexp.MustCompile(`^then-(\d+)-(prompt\.txt|args|cwd)$`)

// stepSpec is one run of the harness the recording holds: the first is the run's own prompt, the
// others come from setup/then-NN-*, in name order.
type stepSpec struct {
	prompt string
	args   []string // the words given before the prompt, "<SESSION>" being the first run's session
	cwd    string   // a directory beside the repository to run from; empty: the repository
}

func stepSpecs(setup string) []stepSpec {
	specs := []stepSpec{{prompt: strings.TrimSpace(readFile(filepath.Join(setup, "prompt.txt")))}}
	prompts, _ := filepath.Glob(filepath.Join(setup, "then-*-prompt.txt"))
	sort.Strings(prompts)
	for _, p := range prompts {
		base := strings.TrimSuffix(p, "prompt.txt")
		s := stepSpec{prompt: strings.TrimSpace(readFile(p)), cwd: strings.TrimSpace(readFile(base + "cwd"))}
		for _, a := range strings.Split(readFile(base+"args"), "\n") {
			if a = strings.TrimSpace(a); a != "" {
				s.args = append(s.args, a)
			}
		}
		specs = append(specs, s)
	}
	return specs
}

// setupFileOK is whether the adapter installs the setup file.
// The one run option installed is a token limit that makes the harness compact the session: the mock
// has no tokens, so the script compacts where the harness did (the rollout's compacted records).
func setupFileOK(setup, name string) bool {
	if name == "args" {
		a := strings.TrimSpace(readFile(filepath.Join(setup, name)))
		return a == "-c\nmodel_auto_compact_token_limit=3000" || a == ephemeralFlag
	}
	// interrupt-after: the run was sent SIGINT; the replay sends it when the command has started (interrupt.go)
	return name == "hooks.json" || name == "hook.sh" || name == "prompt.txt" || name == "interrupt-after" || stepFile.MatchString(name)
}

// threadsOf are the threads the stream starts, in order: the thread each run of the harness
// worked in (a resume names the first one again, a fork starts another).
func threadsOf(stream []map[string]any) (out []string) {
	for _, l := range stream {
		if l["type"] == "thread.started" {
			id, _ := l["thread_id"].(string)
			out = append(out, id)
		}
	}
	return out
}

// promptAt is where, from `from` on, the user message of the prompt is in a rollout, or -1.
func promptAt(records []map[string]any, prompt string, from int) int {
	for i := from; i < len(records); i++ {
		p, _ := records[i]["payload"].(map[string]any)
		if records[i]["type"] != "response_item" || p["type"] != "message" || p["role"] != "user" {
			continue
		}
		content, _ := p["content"].([]any)
		for _, c := range content {
			if m, _ := c.(map[string]any); m != nil && strings.TrimSpace(fmt.Sprint(m["text"])) == prompt {
				return i
			}
		}
	}
	return -1
}

// stepRecords are the records each run of the harness made. A thread worked in more than once holds
// the runs one after the other, each after the user message of its own prompt; a forked thread starts
// with a copy of the history, so a run's own prompt is the one searched for, in order.
func stepRecords(specs []stepSpec, threads []string, rollouts map[string][]map[string]any) ([][]map[string]any, error) {
	if len(threads) != len(specs) {
		return nil, fmt.Errorf("the setup has %d runs of the harness and the stream starts %d threads", len(specs), len(threads))
	}
	at := make([]int, len(specs)) // where each run's prompt message is (the first run's has none)
	last := map[string]int{}
	for i, sp := range specs {
		recs, ok := rollouts[threads[i]]
		if !ok {
			return nil, fmt.Errorf("run %d worked in thread %s, which has no recorded rollout", i, threads[i])
		}
		at[i] = -1
		if i > 0 {
			from := 0
			if l, seen := last[threads[i]]; seen {
				from = l + 1
			}
			if at[i] = promptAt(recs, sp.prompt, from); at[i] < 0 {
				return nil, fmt.Errorf("run %d: its prompt is not in the rollout of thread %s", i, threads[i])
			}
		}
		last[threads[i]] = at[i]
	}
	out := make([][]map[string]any, len(specs))
	for i := range specs {
		recs, end := rollouts[threads[i]], len(rollouts[threads[i]])
		for j := i + 1; j < len(specs); j++ {
			if threads[j] == threads[i] {
				end = at[j]
				break
			}
		}
		out[i] = recs[at[i]+1 : end]
	}
	return out, nil
}

// thenSteps are the later runs in unified form. What a later run starts (a sub-agent) is not replayed yet.
func thenSteps(specs []stepSpec, records [][]map[string]any) ([]core.Step, error) {
	var out []core.Step
	for i, sp := range specs {
		agent, err := modelTurns(records[i], nil)
		if err != nil {
			return nil, err
		}
		for _, c := range agent.Calls {
			if c.Tool == core.ToolSpawn || c.Tool == core.ToolWait {
				return nil, fmt.Errorf("a later run of the harness starts sub-agents: not replayed yet")
			}
		}
		out = append(out, core.Step{Prompt: sp.prompt, Args: sp.args, Cwd: sp.cwd, Agent: agent})
	}
	return out, nil
}

func contains(list []string, s string) bool {
	for _, e := range list {
		if e == s {
			return true
		}
	}
	return false
}
