package replay

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"go.yaml.in/yaml/v3"

	core "github.com/sloprail/harness-mocks/internal/replay"
)

// Unbuildable is the core's: what of a recording the adapter cannot reproduce.
type Unbuildable = core.Unbuildable

func unbuildable(err error) error { return &core.Unbuildable{Reason: err.Error()} }

// Load reads the recorded run in runDir into the unified form; an *Unbuildable says what cannot be reproduced.
func (Adapter) Load(runDir string) (core.Recording, error) {
	if fi, err := os.Stat(runDir); err != nil || !fi.IsDir() {
		return core.Recording{}, fmt.Errorf("%s is not a recorded run", runDir)
	}
	setup, sample := filepath.Join(runDir, "setup"), sampleDir(runDir)
	run, err := readRun(filepath.Join(runDir, "run.yaml"))
	if err != nil {
		return core.Recording{}, unbuildable(err)
	}
	cmdline, err := cmdFlags(run.Command)
	if err != nil {
		return core.Recording{}, unbuildable(err)
	}
	entries, _ := os.ReadDir(setup)
	for _, e := range entries {
		if !setupFileOK(setup, e.Name()) {
			return core.Recording{}, unbuildable(fmt.Errorf("the setup has %s, which the adapter does not install", e.Name()))
		}
	}
	if sample == "" {
		return core.Recording{}, unbuildable(fmt.Errorf("no sample was recorded"))
	}
	paths, _ := filepath.Glob(filepath.Join(sample, "transcript", "*.jsonl"))
	if ephemeral(setup) { // no rollout is kept: the turns are read off the stream
		return loadEphemeral(runDir, setup, sample, cmdline)
	}
	if len(paths) == 0 && exitOf(sample) != "0" && exitOf(sample) != "" { // the harness refused the run: what is replayed is the refusal
		return refusedRun(runDir, setup, setupOf(setup, sample, cmdline)), nil
	}
	if len(paths) == 0 {
		return core.Recording{}, unbuildable(fmt.Errorf("no rollout was recorded: the model's turns are unknown"))
	}
	stream, err := readJSONL(filepath.Join(sample, "stream.jsonl"))
	if err != nil {
		return core.Recording{}, err
	}
	if _, err := parseHookLog(readFile(filepath.Join(sample, "payloads.jsonl"))); err != nil {
		return core.Recording{}, err
	}
	threads := threadsOf(stream)
	if len(threads) == 0 {
		return core.Recording{}, unbuildable(fmt.Errorf("the stream starts no thread"))
	}
	main := threads[0]
	rollouts := map[string][]map[string]any{} // every rollout, by thread id
	for _, p := range paths {
		rollout, err := readJSONL(p)
		if err != nil {
			return core.Recording{}, err
		}
		rollouts[threadOf(p)] = rollout
	}
	specs := stepSpecs(setup)
	records, err := stepRecords(specs, threads, rollouts)
	if err != nil {
		return core.Recording{}, unbuildable(err)
	}
	subs := map[string][]map[string]any{} // the sub-agents' rollouts, by thread id: those no run of the harness worked in
	for thread, rollout := range rollouts {
		if !contains(threads, thread) {
			subs[thread] = rollout
		}
	}
	agent, err := modelTurns(records[0], spawnReceipts(stream, main))
	if err != nil {
		return core.Recording{}, unbuildable(err)
	}
	then, err := thenSteps(specs[1:], records[1:])
	if err != nil {
		return core.Recording{}, unbuildable(err)
	}
	if err := attachSubs(&agent, subs); err != nil {
		return core.Recording{}, unbuildable(err)
	}
	for thread := range subs {
		return core.Recording{}, unbuildable(fmt.Errorf("the rollout of thread %s is no spawn_agent's of any agent", thread))
	}
	return core.Recording{
		Dir:    runDir,
		Prompt: specs[0].prompt,
		Setup:  setupOf(setup, sample, cmdline),
		Agent:  agent,
		Then:   then,
	}, nil
}

// run is a recorded run's run.yaml.
type run struct {
	Version string `yaml:"version"`
	Command string `yaml:"command"`
}

func readRun(path string) (run, error) {
	var r run
	b, err := os.ReadFile(path)
	if err != nil {
		return r, fmt.Errorf("run.yaml: %w", err)
	}
	if err := yaml.Unmarshal(b, &r); err != nil {
		return r, fmt.Errorf("run.yaml: %w", err)
	}
	return r, nil
}

// the rollout of a thread is rollout-<start time>-<thread id>.jsonl
var reRollout = regexp.MustCompile(`^rollout-\d{4}-\d{2}-\d{2}T[\d-]+-([0-9a-f-]{36})\.jsonl$`)

// threadOf is the thread id a rollout file's name carries, empty if it is not a rollout's name.
func threadOf(path string) string {
	if m := reRollout.FindStringSubmatch(filepath.Base(path)); m != nil {
		return m[1]
	}
	return ""
}
