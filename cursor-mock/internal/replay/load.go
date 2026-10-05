package replay

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"

	core "github.com/sloprail/harness-mocks/internal/replay"
)

// Unbuildable is the core's: what of a recording the adapter cannot reproduce.
type Unbuildable = core.Unbuildable

func unbuildable(format string, args ...any) error {
	return &core.Unbuildable{Reason: fmt.Sprintf(format, args...)}
}

// the command line a replayable recording was made with: the mock is run with
// the equivalent flags, and a recording made another way is not replayed by this
// adapter. A setup with a no-force file leaves --force off.
const (
	forcedCommand   = "cursor-agent -p --force --trust --model auto --output-format stream-json"
	unforcedCommand = "cursor-agent -p --trust --model auto --output-format stream-json"
)

// the files of a recording's setup the adapter installs, besides the hook
// scripts (*.sh); any other (extra flags, a preparation script, later steps) is
// something of the recording the adapter cannot reproduce yet
var installed = map[string]bool{"hooks.json": true, "user-hooks.json": true, "prompt.txt": true, "no-force": true, "env": true, "symlink": true, "prepare.sh": true, "args": true}

// sampleDir is the latest sample of the run in dir; empty when it has none.
func sampleDir(dir string) string {
	samples, _ := filepath.Glob(filepath.Join(dir, "samples", "*"))
	if len(samples) == 0 {
		return ""
	}
	sort.Strings(samples)
	return samples[len(samples)-1]
}

// Load reads the recorded run in runDir (run.yaml, setup/, samples/) into the
// unified form: the main session's calls from its transcript, with the
// sub-agents it started attached. Only the transcripts' assistant records are
// read (the model's turns); a replay compares the stream and the hook payloads,
// whose files are only checked to be readable here. An *Unbuildable says what
// the adapter cannot reproduce.
func (a Adapter) Load(runDir string) (core.Recording, error) {
	if fi, err := os.Stat(runDir); err != nil || !fi.IsDir() {
		return core.Recording{}, fmt.Errorf("%s is not a recorded run", runDir)
	}
	setup, sample := filepath.Join(runDir, "setup"), a.sampleDir(runDir)
	command, err := recordedCommand(filepath.Join(runDir, "run.yaml"))
	if err != nil {
		return core.Recording{}, unbuildable("run.yaml: %v", err)
	}
	rec := core.Recording{Dir: runDir, Prompt: strings.TrimRight(readFile(filepath.Join(setup, "prompt.txt")), "\n"), Setup: map[string]string{}}
	entries, _ := os.ReadDir(setup)
	for _, e := range entries {
		name := e.Name()
		switch {
		case installed[name]:
			rec.Setup[name] = readFile(filepath.Join(setup, name))
		case strings.HasSuffix(name, ".sh"):
			rec.Setup[name] = readFile(filepath.Join(setup, name))
		default:
			return core.Recording{}, unbuildable("the setup has %s, which the adapter does not install", name)
		}
	}
	if _, err := flagWords(rec.Setup["args"]); err != nil {
		return core.Recording{}, err
	}
	if _, noForce := rec.Setup["no-force"]; noForce != (command == unforcedCommand) || command != forcedCommand && command != unforcedCommand {
		return core.Recording{}, unbuildable("recorded with another command line: %q", command)
	}
	if sample == "" {
		return core.Recording{}, unbuildable("no sample was recorded")
	}
	if code := strings.TrimSpace(readFile(filepath.Join(sample, "exit.txt"))); code != "0" {
		return core.Recording{}, unbuildable("cursor-agent exited %q: the adapter replays runs that end well", code)
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
		return core.Recording{}, unbuildable("the stream names no session")
	}
	dir := filepath.Join(sample, "transcript")
	records, err := readJSONL(filepath.Join(dir, session, session+".jsonl"))
	if err != nil {
		return core.Recording{}, err
	}
	if len(records) == 0 {
		return core.Recording{}, unbuildable("no transcript of the main session was recorded: the model's turns are unknown")
	}
	main, err := modelTurns(records)
	if err != nil {
		return core.Recording{}, unbuildable("%v", err)
	}
	convs, err := conversations(dir, session)
	if err != nil {
		return core.Recording{}, err
	}
	if rec.Agent, err = attach(main, &convs); err != nil {
		return core.Recording{}, err
	}
	if len(convs) > 0 {
		return core.Recording{}, unbuildable("a sub-agent whose starting call is in no transcript")
	}
	describeMCPCalls(&rec.Agent, stream)
	return rec, nil
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

// recordedCommand is the command line run.yaml says the run was made with.
func recordedCommand(path string) (string, error) {
	var r struct {
		Command string `yaml:"command"`
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if err := yaml.Unmarshal(b, &r); err != nil {
		return "", err
	}
	return r.Command, nil
}

// describeMCPCalls gives each MCP call of the main agent the description the
// model wrote for it, which only the stream holds (the call's started frame
// carries it beside its args, and the transcript's block does not): the calls
// and the stream's mcpToolCall frames are in the same order.
func describeMCPCalls(a *core.Agent, stream []map[string]any) {
	var descriptions []string
	for _, f := range stream {
		if f["type"] != "tool_call" || f["subtype"] != "started" {
			continue
		}
		tc, _ := f["tool_call"].(map[string]any)
		if body, ok := tc["mcpToolCall"].(map[string]any); ok {
			d, _ := body["description"].(string)
			descriptions = append(descriptions, d)
		}
	}
	i := 0
	for n := range a.Calls {
		c := &a.Calls[n]
		if c.Tool != core.ToolMCP {
			continue
		}
		if i < len(descriptions) && descriptions[i] != "" {
			c.Input["description"] = descriptions[i]
		}
		i++
	}
}
