// Package replay is the codex mock's side of replaying a recording: it turns
// a recorded run into the scenario script the mock takes (the model's turns,
// read from the recorded rollouts), runs the mock on the run's own setup, and
// has the shared core (internal/replay) compare the mock's whole event stream
// and hook payloads with the recording's.
package replay

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	core "github.com/sloprail/harness-mocks/internal/replay"
)

// Script is what Build generated, printed for debugging: the main script and
// each sub-agent's.
func Script(runDir string) (string, error) {
	rec, err := Load(runDir)
	if err != nil {
		return "", err
	}
	s, err := Build(rec)
	if err != nil {
		return "", err
	}
	out := "# main\n" + s.Script
	for name, body := range s.Scripts {
		out += "\n# " + name + "\n" + body
	}
	return out, nil
}

// Run replays the recording in runDir: the mock binary is run on the
// scenario Build makes, and what differs from the recording is returned; empty
// is a green replay. A recording the adapter cannot build is an *Unbuildable.
func Run(mock, runDir string) (diff string, err error) {
	rec, err := Load(runDir)
	if err != nil {
		return "", err
	}
	s, err := Build(rec)
	if err != nil {
		return "", err
	}
	root, err := os.MkdirTemp("", "codex-replay-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(root)
	root, _ = filepath.EvalSymlinks(root)
	repo, home, tmp := filepath.Join(root, "repo"), filepath.Join(root, "home", ".codex"), filepath.Join(root, "tmp")
	for _, d := range []string{repo, home, tmp} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return "", err
		}
	}
	// the scratch repository a recording was made in: one empty commit, "init" (capture.sh)
	for _, args := range [][]string{{"init", "-q"}, {"-c", "user.name=replay", "-c", "user.email=replay@example.invalid", "-c", "commit.gpgsign=false", "commit", "-q", "--allow-empty", "-m", "init"}} {
		if out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput(); err != nil {
			return "", fmt.Errorf("git %s: %w: %s", args[0], err, out)
		}
	}
	scriptsAt := filepath.Join(root, "scripts")
	if err := os.MkdirAll(scriptsAt, 0o755); err != nil {
		return "", err
	}
	for name, body := range s.Scripts {
		if err := os.WriteFile(filepath.Join(scriptsAt, name), []byte(strings.ReplaceAll(strings.ReplaceAll(body, scriptsDir, scriptsAt), runPlaceholder, repo)), 0o755); err != nil {
			return "", err
		}
	}
	s.Script = strings.ReplaceAll(strings.ReplaceAll(s.Script, scriptsDir, scriptsAt), runPlaceholder, repo)
	if s.HooksJSON != "" {
		if err := os.WriteFile(filepath.Join(home, "hooks.json"), []byte(s.HooksJSON), 0o644); err != nil {
			return "", err
		}
	}
	if s.ProjectHooksJSON != "" {
		if err := os.MkdirAll(filepath.Join(repo, ".codex"), 0o755); err != nil {
			return "", err
		}
		if err := os.WriteFile(filepath.Join(repo, ".codex", "hooks.json"), []byte(s.ProjectHooksJSON), 0o644); err != nil {
			return "", err
		}
	}
	for name, body := range s.Files {
		if err := os.WriteFile(filepath.Join(repo, name), []byte(body), 0o755); err != nil {
			return "", err
		}
	}
	script := filepath.Join(root, "scenario.sh")
	if err := os.WriteFile(script, []byte(s.Script), 0o755); err != nil {
		return "", err
	}

	// hermetic but for the tools (sh, git, jq) the hooks use: no CODEX_* or CLAUDE* variable of ours reaches the mock
	env := []string{"CODEX_HOME=" + home, "TMPDIR=" + tmp, "HOOK_LOG=" + filepath.Join(tmp, "hook.log")}
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "CODEX") && !strings.HasPrefix(kv, "CLAUDE") && !strings.HasPrefix(kv, "TMPDIR=") {
			env = append(env, kv)
		}
	}
	args := append([]string{"exec", "--dangerously-bypass-hook-trust", "--json", "--skip-git-repo-check", "--script", script, "-m", "mock-model"}, s.Args...)
	cmd := exec.Command(mock, append(args, s.Prompt)...)
	cmd.Dir, cmd.Env = repo, append(env, s.Env...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return fmt.Sprintf("the mock failed: %v: %s", err, errb.String()), nil
	}
	hookLog, _ := os.ReadFile(filepath.Join(tmp, "hook.log"))

	// one canonicalisation per side, the event stream first: it names the ids in a fixed order
	rules := Rules(repo, root)
	wantC, gotC := core.New(rules), core.New(rules)
	wantStream := wantC.Lines(jsonLines(readFile(filepath.Join(rec.Sample, "stream.jsonl"))))
	gotStream := gotC.Lines(jsonLines(out.String()))
	wantHooks := wantC.Lines(jsonLines(readFile(filepath.Join(rec.Sample, "payloads.jsonl"))))
	gotHooks := gotC.Lines(jsonLines(string(hookLog)))
	// hooks of one event run at the same time, so the order they log in is not the behaviour
	return core.Diff("event stream", wantStream, gotStream) +
		core.Diff("hook payloads", core.Sorted(wantHooks), core.Sorted(gotHooks)), nil
}
