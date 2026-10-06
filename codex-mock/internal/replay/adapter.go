// Package replay is the codex mock's replay adapter: every codex-specific
// decision of replaying a recording is here (reading the rollouts into turns,
// writing the mock's scenario script, running the mock, which fields are not
// behaviour). The shared core (internal/replay) speaks only the unified format
// and compares what the adapter normalises.
package replay

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sloprail/harness-mocks/internal/procexec"
	core "github.com/sloprail/harness-mocks/internal/replay"
)

// Adapter is the codex harness's side of a replay (core.Adapter): it reads a
// codex recording into the unified form (Load), turns that into the scenario the
// codex mock takes (Denormalize), runs the mock, and normalises both outputs by
// codex's own rules (Rules).
type Adapter struct {
	// Environ is the environment the mock and its children inherit; the entrypoint reads it once and passes it down.
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
	s := Denormalize(rec)
	out := "# main\n" + s.Script
	for name, body := range s.Scripts {
		out += "\n# " + name + "\n" + body
	}
	return out, nil
}

// Replay runs the mock on rec's scenario in a hermetic repository and returns the recording's output and the mock's.
func (a Adapter) Replay(mock string, rec core.Recording) (want, got core.Observed, err error) {
	if rec.Setup["refused-flag"] != "" { // a flag the mock refuses: replayed as the check that it does
		return core.Observed{Checked: true}, core.Observed{Checked: true}, checkRefusal(mock, rec, a.Environ)
	}
	s := Denormalize(rec)
	root, err := os.MkdirTemp("", "codex-replay-*")
	if err != nil {
		return want, got, err
	}
	defer os.RemoveAll(root)
	root, _ = filepath.EvalSymlinks(root)
	repo, home, tmp := filepath.Join(root, "repo"), filepath.Join(root, "home", ".codex"), filepath.Join(root, "tmp")
	for _, d := range []string{repo, home, tmp} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return want, got, err
		}
	}
	// hermetic as a capture is (env -i PATH HOME CODEX_HOME USER LANG TERM TMPDIR HOOK_LOG): nothing else of ours reaches the mock
	env := []string{"HOME=" + filepath.Dir(home), "CODEX_HOME=" + home, "TMPDIR=" + tmp, "HOOK_LOG=" + filepath.Join(tmp, "hook.log")}
	for _, kv := range a.Environ {
		if k, _, _ := strings.Cut(kv, "="); k == "PATH" || k == "USER" || k == "LANG" || k == "TERM" {
			env = append(env, kv)
		}
	}
	env = append(env, s.Env...) // what the recorded run was also given (setup/env)
	ctx := context.Background()
	// the scratch repository a recording was made in: branch main, one empty commit, "init" (capture.sh; the host's default branch name is not behaviour)
	for _, argv := range gitSetup(s.NoGit) {
		if res, err := procexec.Run(ctx, procexec.Spec{Argv: append([]string{"git", "-C", repo}, argv...), Env: env}); err != nil || res.ExitCode != 0 {
			return want, got, fmt.Errorf("git %s: %v %s", argv[len(argv)-1], err, res.Stderr)
		}
	}
	if s.HooksJSON != "" { // a run with no hooks has none: an empty file is not a hooks file
		if err := os.WriteFile(filepath.Join(home, "hooks.json"), []byte(s.HooksJSON), 0o644); err != nil {
			return want, got, err
		}
	}
	if err := writeProjectHooks(repo, s.ProjectHooksJSON); err != nil {
		return want, got, err
	}
	for name, body := range s.Files {
		if err := os.WriteFile(filepath.Join(repo, name), []byte(inRepo(name, body, repo)), 0o755); err != nil {
			return want, got, err
		}
	}
	if s.Prepare != "" {
		if err := prepare(ctx, s.Prepare, root, repo, home, env); err != nil {
			return want, got, err
		}
	}
	scriptsAt := filepath.Join(root, "scripts")
	if err := os.MkdirAll(scriptsAt, 0o755); err != nil {
		return want, got, err
	}
	for name, body := range s.Scripts {
		if err := os.WriteFile(filepath.Join(scriptsAt, name), []byte(scriptText(body, scriptsAt, repo)), 0o755); err != nil {
			return want, got, err
		}
	}
	stdout, err := runSteps(ctx, mock, s, repo, root, scriptsAt, env)
	if err != nil {
		return want, got, err
	}
	hookLog, _ := os.ReadFile(filepath.Join(tmp, "hook.log"))

	// the mock's output is compared with every sample of the recording, each under a header
	mockStream, err := parseStream(stdout, s.CmdFlags)
	if err != nil {
		return want, got, fmt.Errorf("the mock's stream: %w", err)
	}
	mockHooks, err := parseHookLog(string(hookLog))
	if err != nil {
		return want, got, fmt.Errorf("the mock's hook log: %w", err)
	}
	for _, sample := range sampleDirs(rec.Dir) {
		recStream, err := eventStream(sample, s.CmdFlags)
		if err != nil {
			return want, got, err
		}
		recHooks, err := parseHookLog(readFile(filepath.Join(sample, "payloads.jsonl")))
		if err != nil {
			return want, got, err
		}
		w, g := observe(Rules(repo, root), asyncEvents(s.HooksJSON), recStream, recHooks, mockStream, mockHooks)
		header := "sample " + filepath.Base(sample)
		want.Events, got.Events = append(append(want.Events, header), w.Events...), append(append(got.Events, header), g.Events...)
		want.Hooks, got.Hooks = append(append(want.Hooks, header), w.Hooks...), append(append(got.Hooks, header), g.Hooks...)
	}
	return want, got, nil
}
