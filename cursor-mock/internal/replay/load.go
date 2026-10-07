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
var installed = map[string]bool{"hooks.json": true, "user-hooks.json": true, "prompt.txt": true, "no-force": true, "env": true, "tui.yaml": true, "symlink": true, "prepare.sh": true, "args": true}

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
// sub-agents it started attached (only its assistant records, the model's turns,
// are read). An *Unbuildable says what the adapter cannot reproduce.
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
		case installed[name] || stepFile(name):
			rec.Setup[name] = readFile(filepath.Join(setup, name))
		case strings.HasSuffix(name, ".sh"):
			rec.Setup[name] = readFile(filepath.Join(setup, name))
		default:
			return core.Recording{}, unbuildable("the setup has %s, which the adapter does not install", name)
		}
	}
	if _, err := flagWords(rec.Setup["args"], false); err != nil {
		return core.Recording{}, err
	}
	if !commandOK(rec.Setup, command) {
		return core.Recording{}, unbuildable("recorded with another command line: %q", command)
	}
	if sample == "" {
		return core.Recording{}, unbuildable("no sample was recorded")
	}
	prompts, exits, err := stepsOf(&rec, sample)
	if err != nil {
		return core.Recording{}, err
	}
	stream, err := readJSONL(filepath.Join(sample, "stream.jsonl"))
	if err != nil {
		return core.Recording{}, err
	}
	payloads, err := readJSONL(filepath.Join(sample, "payloads.jsonl"))
	if err != nil {
		return core.Recording{}, err
	}
	thoughts, err := thoughtsOf(payloads)
	if err != nil {
		return core.Recording{}, err
	}
	session := sessionOfRun(stream, payloads)
	if session == "" {
		return core.Recording{}, unbuildable("the run names no session")
	}
	dir := filepath.Join(sample, "transcript")
	if refused, ok := untranscribed(rec, dir); ok { // a refused prompt: no turn, no transcript
		return refused, nil
	}
	files, err := transcriptsOf(dir, session)
	if err != nil {
		return core.Recording{}, err
	}
	if marked := compactionsOf(payloads); len(marked) > 0 {
		if len(files) != 1 || len(prompts) != 1 {
			return core.Recording{}, unbuildable("the harness compacted in a run of several steps: the compactions are not told apart by step")
		}
		for name, records := range files {
			if files[name], err = markCompactions(records, marked); err != nil {
				return core.Recording{}, err
			}
		}
	}
	agents, err := modelSteps(files, rec.Setup, prompts, exits, thoughts[session])
	if err != nil {
		return core.Recording{}, err
	}
	main := agents[0]
	delete(thoughts, session)
	convs, err := conversations(dir, session, thoughts)
	if err != nil {
		return core.Recording{}, err
	}
	if rec.Agent, err = attach(main, &convs); err != nil {
		return core.Recording{}, err
	}
	if len(convs) > 0 {
		return core.Recording{}, unbuildable("a sub-agent whose starting call is in no transcript")
	}
	if err := addLater(&rec, prompts, agents); err != nil {
		return core.Recording{}, err
	}
	if len(thoughts) > 0 {
		return core.Recording{}, unbuildable("the model thought in a conversation that has no transcript")
	}
	for name, body := range harnessFiles(stream) {
		rec.Setup[name] = body
	}
	if err := nameIDsOfRun(&rec, stream, payloads, session); err != nil {
		return core.Recording{}, err
	}
	if err := nameShellIDs(&rec.Agent, stream, session); err != nil {
		return core.Recording{}, err
	}
	describeMCPCalls(&rec.Agent, stream)
	return rec, nil
}
