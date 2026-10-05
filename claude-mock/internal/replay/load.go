package replay

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	core "github.com/sloprail/harness-mocks/internal/replay"
)

// Unbuildable is the core's: what of a recording the adapter cannot reproduce.
type Unbuildable = core.Unbuildable

func unbuildable(err error) error { return &core.Unbuildable{Reason: err.Error()} }

// the command line every replayable recording was made with: the mock is run
// with the equivalent flags, and a recording made another way is not replayed
// by this adapter.
const standardCommand = "claude -p --model haiku --dangerously-skip-permissions --output-format stream-json --verbose"

// the files of a recording's setup the adapter installs; any other (extra
// flags, later steps, a preparation script, environment) is something of the
// recording the adapter cannot reproduce yet
var installed = map[string]bool{"hook.sh": true, "prompt.txt": true, "settings.json": true, "prepare.sh": true, "args": true}

// sampleDir is the latest sample of the run in dir, the one the model's turns
// and the stream's session are read from; empty when it has none.
func sampleDir(dir string) string {
	samples, _ := filepath.Glob(filepath.Join(dir, "samples", "*"))
	if len(samples) == 0 {
		return ""
	}
	sort.Strings(samples)
	return samples[len(samples)-1]
}

// run is a recorded run's run.yaml.
type run struct {
	Version string `yaml:"version"`
	Command string `yaml:"command"`
}

// Load reads the recorded run in runDir (run.yaml, setup/, samples/) into the
// unified form: the main agent's calls from its transcript, with the
// sub-agents it spawned attached. Only the transcripts' assistant records are
// read (the model's turns); what else they hold (user, system and attachment
// records) is not compared: a replay compares the stream and the hook payloads,
// whose files are only checked to be readable here. An *Unbuildable says what
// the adapter cannot reproduce.
func (a Adapter) Load(runDir string) (core.Recording, error) {
	return a.LoadSample(runDir, sampleDir(runDir))
}

// LoadSample is Load for one sample of the run: the model's turns are those of
// that sample's transcripts.
func (Adapter) LoadSample(runDir, sample string) (core.Recording, error) {
	if fi, err := os.Stat(runDir); err != nil || !fi.IsDir() {
		return core.Recording{}, fmt.Errorf("%s is not a recorded run", runDir)
	}
	setup := filepath.Join(runDir, "setup")
	r, err := readRun(filepath.Join(runDir, "run.yaml"))
	if err != nil {
		return core.Recording{}, unbuildable(err)
	}
	if r.Command != standardCommand {
		return core.Recording{}, unbuildable(fmt.Errorf("recorded with another command line: %q", r.Command))
	}
	entries, _ := os.ReadDir(setup)
	for _, e := range entries {
		if !installed[e.Name()] {
			return core.Recording{}, unbuildable(fmt.Errorf("the setup has %s, which the adapter does not install", e.Name()))
		}
	}
	if sample == "" {
		return core.Recording{}, unbuildable(fmt.Errorf("no sample was recorded"))
	}
	code := strings.TrimSpace(readFile(filepath.Join(sample, "exit.txt")))
	if code != "0" && code != "1" {
		return core.Recording{}, unbuildable(fmt.Errorf("claude exited %q: the adapter replays runs that end with 0 or 1", code))
	}
	stream, err := readJSONL(filepath.Join(sample, "stream.jsonl"))
	if err != nil {
		return core.Recording{}, err
	}
	if _, err := readJSONL(filepath.Join(sample, "payloads.jsonl")); err != nil {
		return core.Recording{}, err
	}
	session := sessionOf(stream)
	if session == "" {
		return core.Recording{}, unbuildable(fmt.Errorf("the stream names no session"))
	}
	mainRecords, err := readJSONL(filepath.Join(sample, "transcript", session+".jsonl"))
	if err != nil {
		return core.Recording{}, err
	}
	if len(mainRecords) == 0 {
		return core.Recording{}, unbuildable(fmt.Errorf("no transcript of the main session was recorded: the model's turns are unknown"))
	}
	main, err := modelTurns(mainRecords)
	if err != nil {
		return core.Recording{}, unbuildable(err)
	}
	subs, err := subagentTurns(filepath.Join(sample, "transcript", session, "subagents"))
	if err != nil {
		return core.Recording{}, err
	}
	agent := attachSubagents(main, subs)
	if len(subs) > 0 {
		return core.Recording{}, unbuildable(fmt.Errorf("a sub-agent whose starting call is in no transcript"))
	}
	args, err := parseArgs(readFile(filepath.Join(setup, "args")))
	if err != nil {
		return core.Recording{}, err
	}
	return core.Recording{
		Dir:    runDir,
		Prompt: strings.TrimSpace(readFile(filepath.Join(setup, "prompt.txt"))),
		Setup: map[string]string{
			"settings.json": readFile(filepath.Join(setup, "settings.json")),
			"hook.sh":       readFile(filepath.Join(setup, "hook.sh")),
			"prepare.sh":    readFile(filepath.Join(setup, "prepare.sh")),
			"args":          strings.Join(args, "\n"),
			"exit":          code,
		},
		Agent: agent,
	}, nil
}

// sessionOf is the session id the stream names.
func sessionOf(stream []map[string]any) string {
	for _, f := range stream {
		if id, _ := f["session_id"].(string); id != "" {
			return id
		}
	}
	return ""
}
