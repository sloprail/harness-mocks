package replay

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	core "github.com/sloprail/harness-mocks/internal/replay"
)

// Unbuildable is the core's: what of a recording the adapter cannot reproduce.
type Unbuildable = core.Unbuildable

func unbuildable(err error) error { return &core.Unbuildable{Reason: err.Error()} }

// the command line every replayable recording was made with: the mock is run
// with the equivalent flags, and a recording made another way (a resume, a -c
// override, an output schema) is not replayed by this adapter.
const standardCommand = "codex exec --json --skip-git-repo-check --dangerously-bypass-approvals-and-sandbox --dangerously-bypass-hook-trust -m gpt-5.6-luna"

// sampleDir is the latest sample of the run in dir; empty when it has none.
func sampleDir(dir string) string {
	samples, _ := filepath.Glob(filepath.Join(dir, "samples", "*"))
	if len(samples) == 0 {
		return ""
	}
	return samples[len(samples)-1]
}

// Load reads the recorded run in runDir (run.yaml, setup/, samples/) into the
// unified form: the main agent's calls with the sub-agents it spawned attached,
// in the order they were spawned. An *Unbuildable says what the adapter cannot
// reproduce.
func (Adapter) Load(runDir string) (core.Recording, error) {
	if fi, err := os.Stat(runDir); err != nil || !fi.IsDir() {
		return core.Recording{}, fmt.Errorf("%s is not a recorded run", runDir)
	}
	setup, sample := filepath.Join(runDir, "setup"), sampleDir(runDir)
	cmd := ""
	for _, l := range strings.Split(readFile(filepath.Join(runDir, "run.yaml")), "\n") {
		if v, ok := strings.CutPrefix(l, "command: "); ok {
			cmd = v
		}
	}
	if cmd != standardCommand {
		return core.Recording{}, unbuildable(fmt.Errorf("recorded with another command line: %q", cmd))
	}
	entries, _ := os.ReadDir(setup)
	for _, e := range entries {
		if n := e.Name(); n != "hooks.json" && n != "hook.sh" && n != "prompt.txt" {
			return core.Recording{}, unbuildable(fmt.Errorf("the setup has %s, which the adapter does not install", n))
		}
	}
	if sample == "" {
		return core.Recording{}, unbuildable(fmt.Errorf("no sample was recorded"))
	}
	paths, _ := filepath.Glob(filepath.Join(sample, "transcript", "*.jsonl"))
	if len(paths) == 0 {
		return core.Recording{}, unbuildable(fmt.Errorf("no rollout was recorded: the model's turns are unknown"))
	}
	main := ""
	for _, l := range jsonLines(readFile(filepath.Join(sample, "stream.jsonl"))) {
		if l["type"] == "thread.started" {
			main, _ = l["thread_id"].(string)
		}
	}
	var mainRollout string
	var subs []string
	for _, p := range paths {
		if strings.Contains(p, main) {
			mainRollout = readFile(p)
		} else {
			subs = append(subs, readFile(p))
		}
	}
	if mainRollout == "" {
		return core.Recording{}, unbuildable(fmt.Errorf("no rollout of the main thread"))
	}
	agent, err := modelTurns(mainRollout)
	if err != nil {
		return core.Recording{}, unbuildable(err)
	}
	n := 0
	for i := range agent.Calls {
		if agent.Calls[i].Tool != core.ToolSpawn {
			continue
		}
		if n >= len(subs) {
			return core.Recording{}, unbuildable(fmt.Errorf("a spawn_agent call with no recorded sub-agent rollout"))
		}
		sub, err := modelTurns(subs[n])
		if err != nil {
			return core.Recording{}, unbuildable(fmt.Errorf("sub-agent: %w", err))
		}
		agent.Calls[i].Sub = &sub
		n++
	}
	return core.Recording{
		Dir:    runDir,
		Prompt: strings.TrimSpace(readFile(filepath.Join(setup, "prompt.txt"))),
		Setup: map[string]string{
			"hooks.json": readFile(filepath.Join(setup, "hooks.json")),
			"hook.sh":    readFile(filepath.Join(setup, "hook.sh")),
		},
		Agent: agent,
	}, nil
}
