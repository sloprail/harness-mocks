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

// runMock runs the mock on rec's scenario in a hermetic repository, laid out as
// a capture lays out its own (the hook log, the transcripts' home, the temp
// root), and returns what it streamed and the hook payloads it logged, with the
// repository's and the temp root's paths (for scrubbing them).
func (a Adapter) runMock(mock string, rec core.Recording) (stream, hooks []map[string]any, repo, work string, err error) {
	work, err = os.MkdirTemp("", "claude-replay-*")
	if err != nil {
		return nil, nil, "", "", err
	}
	defer os.RemoveAll(work)
	if work, err = filepath.EvalSymlinks(work); err != nil { // canonical, as the capture's paths are
		return nil, nil, "", "", err
	}
	var home, tmp, scripts string
	repo, home, tmp, scripts = filepath.Join(work, "repo"), filepath.Join(work, "home"), filepath.Join(work, "tmp"), filepath.Join(work, "scripts")
	for _, d := range []string{filepath.Join(repo, ".claude"), home, scripts} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return nil, nil, "", "", err
		}
	}
	if err := os.Mkdir(tmp, 0o700); err != nil {
		return nil, nil, "", "", err
	}
	s := Denormalize(rec, scripts)
	// the recording names its run directory <RUN> (as a capture sanitises it): in the calls it is this replay's repository
	fill := strings.NewReplacer("<RUN>", repo, "\\u003cRUN\\u003e", repo) // as the script's JSON writes it too
	s.Script = fill.Replace(s.Script)
	for name, body := range s.Scripts {
		s.Scripts[name] = fill.Replace(body)
	}
	env := a.env(home, tmp, filepath.Join(work, "hook.log"))
	ctx := context.Background()
	for _, argv := range [][]string{
		{"git", "-C", repo, "init", "-q", "-b", "main"},
		{"git", "-C", repo, "-c", "commit.gpgsign=false", "commit", "-q", "--allow-empty", "-m", "init"},
	} {
		if res, err := procexec.Run(ctx, procexec.Spec{Argv: argv, Env: env}); err != nil || res.ExitCode != 0 {
			return nil, nil, "", "", fmt.Errorf("%s: %v %s", strings.Join(argv, " "), err, res.Stderr)
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
		if f.body == "" && filepath.Base(path) == "settings.json" { // a scenario's preparation may write it
			continue
		}
		if err := os.WriteFile(path, []byte(f.body), f.mode); err != nil {
			return nil, nil, "", "", err
		}
	}

	if prep := rec.Setup["prepare.sh"]; prep != "" {
		// a capture runs the scenario's preparation in the scratch repository, before git init and claude
		if err := os.WriteFile(filepath.Join(work, "prepare.sh"), []byte(prep), 0o644); err != nil {
			return nil, nil, "", "", err
		}
		if res, err := procexec.Run(ctx, procexec.Spec{Argv: []string{"sh", filepath.Join(work, "prepare.sh")}, Dir: repo, Env: env}); err != nil || res.ExitCode != 0 {
			return nil, nil, "", "", fmt.Errorf("prepare.sh: %v %s", err, res.Stderr)
		}
	}
	res, err := procexec.Run(ctx, procexec.Spec{
		Argv: append([]string{mock, "-p", "--model", "haiku", "--dangerously-skip-permissions", "--output-format", "stream-json", "--verbose",
			"--script", filepath.Join(work, "main.sh"), "--session-id", sessionID}, append(strings.Fields(rec.Setup["args"]), s.Prompt)...),
		Dir: repo, Env: env, Timeout: 3 * time.Minute})
	if err != nil || res.ExitCode != wantExit(rec) || res.TimedOut {
		return nil, nil, "", "", &core.MockFailure{Detail: fmt.Sprintf("%v (exit %d): %s", err, res.ExitCode, res.Stderr)}
	}
	hookLog, _ := os.ReadFile(filepath.Join(work, "hook.log"))

	mockStream, err := parseJSONL(string(res.Stdout), false)
	if err != nil {
		return nil, nil, "", "", fmt.Errorf("the mock's stream: %w", err)
	}
	mockHooks, err := parseJSONL(string(hookLog), true)
	if err != nil {
		return nil, nil, "", "", fmt.Errorf("the mock's hook log: %w", err)
	}
	return mockStream, mockHooks, repo, work, nil
}
