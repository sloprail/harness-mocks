package replay

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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
var installed = map[string]bool{"hook.sh": true, "prompt.txt": true, "settings.json": true, "prepare.sh": true, "args": true, "then": true, "env": true}

// stepFile is a flat file of a later run (capture.sh: then-<NN>-prompt.txt, then-<NN>-args).
var stepFile = regexp.MustCompile(`^then-\d+-(prompt\.txt|args)$`)

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
		if !installed[e.Name()] && !stepFile.MatchString(e.Name()) {
			return core.Recording{}, unbuildable(fmt.Errorf("the setup has %s, which the adapter does not install", e.Name()))
		}
	}
	if sample == "" {
		return core.Recording{}, unbuildable(fmt.Errorf("no sample was recorded"))
	}
	code := strings.TrimSpace(readFile(filepath.Join(sample, "exit.txt")))
	if codes := strings.Fields(code); len(codes) > 1 { // one line per run of claude
		for _, c := range codes {
			if c != "0" {
				return core.Recording{}, unbuildable(fmt.Errorf("a run of several steps exited %q: the adapter replays steps that end with 0", c))
			}
		}
		code = "0"
	}
	if code != "0" && code != "1" {
		return core.Recording{}, unbuildable(fmt.Errorf("claude exited %q: the adapter replays runs that end with 0 or 1", code))
	}
	stream, err := readJSONL(filepath.Join(sample, "stream.jsonl"))
	if err != nil {
		return core.Recording{}, err
	}
	payloads, err := readJSONL(filepath.Join(sample, "payloads.jsonl"))
	if err != nil {
		return core.Recording{}, err
	}
	agent, then, err := agentsOf(setup, sample, stream, payloads)
	if err != nil {
		return core.Recording{}, err
	}
	if len(then) > 0 && failedResult(stream) != "" {
		return core.Recording{}, unbuildable(fmt.Errorf("a run of several steps whose model API failed"))
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
			"env":           readFile(filepath.Join(setup, "env")),
			"args":          strings.Join(args, "\n"),
			"exit":          code,
			"result":        failedResult(stream),
		},
		Agent: agent,
		Then:  then,
	}, nil
}

// sessionOf is the session id the stream names.
func sessionOf(stream []map[string]any) string {
	for _, f := range stream { // a hook frame ahead of it may name another (a resume's start hook)
		if id, _ := f["session_id"].(string); id != "" && f["type"] == "system" && f["subtype"] == "init" {
			return id
		}
	}
	for _, f := range stream {
		if id, _ := f["session_id"].(string); id != "" {
			return id
		}
	}
	return ""
}
