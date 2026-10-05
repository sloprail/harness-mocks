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

// Replay runs the mock on rec's scenario in a hermetic repository, and returns
// the recording's event stream and hook payloads with the mock's, normalised.
func (a Adapter) Replay(mock string, rec core.Recording) (want, got core.Observed, err error) {
	s := Denormalize(rec)
	sample := sampleDir(rec.Dir)
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
	// hermetic but for the tools (sh, git, jq) the hooks use: no CODEX_* or CLAUDE* variable of ours reaches the mock
	env := []string{"CODEX_HOME=" + home, "TMPDIR=" + tmp, "HOOK_LOG=" + filepath.Join(tmp, "hook.log")}
	for _, kv := range a.Environ {
		if !strings.HasPrefix(kv, "CODEX") && !strings.HasPrefix(kv, "CLAUDE") && !strings.HasPrefix(kv, "TMPDIR=") {
			env = append(env, kv)
		}
	}
	ctx := context.Background()
	// the scratch repository a recording was made in: branch main, one empty commit, "init" (capture.sh; the host's default branch name is not behaviour)
	for _, argv := range [][]string{{"init", "-q", "-b", "main"}, {"-c", "user.name=replay", "-c", "user.email=replay@example.invalid", "-c", "commit.gpgsign=false", "commit", "-q", "--allow-empty", "-m", "init"}} {
		if res, err := procexec.Run(ctx, procexec.Spec{Argv: append([]string{"git", "-C", repo}, argv...), Env: env}); err != nil || res.ExitCode != 0 {
			return want, got, fmt.Errorf("git %s: %v %s", argv[len(argv)-1], err, res.Stderr)
		}
	}
	if err := os.WriteFile(filepath.Join(home, "hooks.json"), []byte(s.HooksJSON), 0o644); err != nil {
		return want, got, err
	}
	for name, body := range s.Files {
		if err := os.WriteFile(filepath.Join(repo, name), []byte(inRepo(name, body, repo)), 0o755); err != nil {
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
	script := filepath.Join(root, "scenario.sh")
	if err := os.WriteFile(script, []byte(scriptText(s.Script, scriptsAt, repo)), 0o755); err != nil {
		return want, got, err
	}

	res, err := procexec.Run(ctx, procexec.Spec{
		Argv: []string{mock, "exec", "--dangerously-bypass-hook-trust", "--json", "--skip-git-repo-check", "--script", script, "-m", "mock-model", s.Prompt},
		Dir:  repo, Env: env})
	if err != nil || res.ExitCode != 0 {
		return want, got, &core.MockFailure{Detail: fmt.Sprintf("%v (exit %d): %s", err, res.ExitCode, res.Stderr)}
	}
	hookLog, _ := os.ReadFile(filepath.Join(tmp, "hook.log"))

	// one canonicalisation per side, the event stream first: it names the ids in a fixed order
	rules := Rules(repo, root)
	wantC, gotC := core.New(rules), core.New(rules)
	recStream, err := readJSONL(filepath.Join(sample, "stream.jsonl"))
	if err != nil {
		return want, got, err
	}
	recHooks, err := readJSONL(filepath.Join(sample, "payloads.jsonl"))
	if err != nil {
		return want, got, err
	}
	mockStream, err := parseJSONL(string(res.Stdout))
	if err != nil {
		return want, got, fmt.Errorf("the mock's stream: %w", err)
	}
	mockHooks, err := parseJSONL(string(hookLog))
	if err != nil {
		return want, got, fmt.Errorf("the mock's hook log: %w", err)
	}
	want.Events, got.Events = wantC.Lines(recStream), gotC.Lines(mockStream)
	want.Hooks, got.Hooks = wantC.Lines(recHooks), gotC.Lines(mockHooks)
	// hooks of one event run at the same time, so the order they log in is not the behaviour:
	// the order of the groups of hooks that run together is (hookorder.go)
	want.Hooks = sortWithinGroups(want.Hooks, concurrentGroups(recHooks))
	got.Hooks = sortWithinGroups(got.Hooks, concurrentGroups(mockHooks))
	return want, got, nil
}
