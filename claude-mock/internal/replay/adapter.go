// Package replay is the claude mock's replay adapter: every claude-specific
// decision of replaying a recording is here (reading the transcripts into
// turns, writing the mock's scenario script, running the mock, which fields
// are not behaviour). The shared core (internal/replay) speaks only the unified
// format and compares what the adapter normalises.
package replay

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/sloprail/harness-mocks/internal/procexec"
	core "github.com/sloprail/harness-mocks/internal/replay"
)

// Adapter is the claude harness's side of a replay (core.Adapter): it reads a
// claude recording into the unified form (Load), turns that into the scenario
// the claude mock takes (Denormalize), runs the mock, and normalises both
// outputs by claude's own rules (Rules).
type Adapter struct {
	// Environ is the environment the entrypoint was started with; only what the hooks' own tools need is passed on.
	Environ []string
}

// Run replays the recording in runDir: the mock binary is run on the scenario
// generated from it, and what differs from the recording is returned; empty is
// a green replay. A recording the adapter cannot build is an *Unbuildable.
func Run(mock, runDir string, environ []string) (string, error) {
	return core.Run(Adapter{Environ: environ}, mock, runDir)
}

// Script is the generated scenario for debugging: the main script and each
// sub-agent's.
func Script(runDir string) (string, error) { return core.Script(Adapter{}, runDir) }

// Script is the scenario Denormalize makes of rec, as text.
func (Adapter) Script(rec core.Recording) (string, error) {
	s := Denormalize(rec, "<scripts>")
	out := "# main\n" + s.Script
	names := make([]string, 0, len(s.Scripts))
	for name := range s.Scripts {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		out += "\n# " + name + "\n" + s.Scripts[name]
	}
	return out, nil
}

// the session id every replay runs under: the recording's own differs in every
// run, and the canonicalisation names it as an id
const sessionID = "00000000-0000-4000-8000-0000000000a1"

// Replay runs the mock on rec's scenario in a hermetic repository, laid out as
// a capture lays out its own (the hook log, the transcripts' home, the temp
// root), and returns the recording's event stream and hook payloads with the
// mock's, normalised.
func (a Adapter) Replay(mock string, rec core.Recording) (want, got core.Observed, err error) {
	sample := sampleDir(rec.Dir)
	work, err := os.MkdirTemp("", "claude-replay-*")
	if err != nil {
		return want, got, err
	}
	defer os.RemoveAll(work)
	if work, err = filepath.EvalSymlinks(work); err != nil { // canonical, as the capture's paths are
		return want, got, err
	}
	repo, home, tmp, scripts := filepath.Join(work, "repo"), filepath.Join(work, "home"), filepath.Join(work, "tmp"), filepath.Join(work, "scripts")
	for _, d := range []string{filepath.Join(repo, ".claude"), home, scripts} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return want, got, err
		}
	}
	if err := os.Mkdir(tmp, 0o700); err != nil {
		return want, got, err
	}
	s := Denormalize(rec, scripts)
	env := a.env(home, tmp, filepath.Join(work, "hook.log"))
	ctx := context.Background()
	for _, argv := range [][]string{
		{"git", "-C", repo, "init", "-q", "-b", "main"},
		{"git", "-C", repo, "-c", "commit.gpgsign=false", "commit", "-q", "--allow-empty", "-m", "init"},
	} {
		if res, err := procexec.Run(ctx, procexec.Spec{Argv: argv, Env: env}); err != nil || res.ExitCode != 0 {
			return want, got, fmt.Errorf("%s: %v %s", strings.Join(argv, " "), err, res.Stderr)
		}
	}
	files := map[string]struct {
		body string
		mode os.FileMode
	}{
		filepath.Join(repo, ".claude", "settings.json"): {s.Settings, 0o644},
		filepath.Join(repo, "hook.sh"):                  {s.Hook, 0o755},
		filepath.Join(work, "main.sh"):                  {s.Script, 0o755},
	}
	for name, body := range s.Scripts {
		files[filepath.Join(scripts, name)] = struct {
			body string
			mode os.FileMode
		}{body, 0o755}
	}
	for path, f := range files {
		if err := os.WriteFile(path, []byte(f.body), f.mode); err != nil {
			return want, got, err
		}
	}

	res, err := procexec.Run(ctx, procexec.Spec{
		Argv: []string{mock, "-p", "--model", "haiku", "--dangerously-skip-permissions", "--output-format", "stream-json", "--verbose",
			"--script", filepath.Join(work, "main.sh"), "--session-id", sessionID, s.Prompt},
		Dir: repo, Env: env, Timeout: time.Minute})
	if err != nil || res.ExitCode != 0 || res.TimedOut {
		return want, got, &core.MockFailure{Detail: fmt.Sprintf("%v (exit %d): %s", err, res.ExitCode, res.Stderr)}
	}
	hookLog, _ := os.ReadFile(filepath.Join(work, "hook.log"))

	recStream, err := readJSONL(filepath.Join(sample, "stream.jsonl"))
	if err != nil {
		return want, got, err
	}
	recHooks, err := readHooks(filepath.Join(sample, "payloads.jsonl"))
	if err != nil {
		return want, got, err
	}
	mockStream, err := parseJSONL(string(res.Stdout), false)
	if err != nil {
		return want, got, fmt.Errorf("the mock's stream: %w", err)
	}
	mockHooks, err := parseJSONL(string(hookLog), true)
	if err != nil {
		return want, got, fmt.Errorf("the mock's hook log: %w", err)
	}

	// one canonicalisation per side, the event stream first: it names the ids in a fixed order
	rules := Rules(repo, work)
	wantC, gotC := core.New(rules), core.New(rules)
	want.Events, got.Events = wantC.Lines(Frames(recStream)), gotC.Lines(Frames(mockStream))
	want.Hooks, got.Hooks = wantC.Lines(recHooks), gotC.Lines(mockHooks)
	return want, got, nil
}
