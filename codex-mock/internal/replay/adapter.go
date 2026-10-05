// Package replay is the codex mock's replay adapter: every codex-specific
// decision of replaying a recording is here (reading the rollouts into turns,
// writing the mock's scenario script, running the mock, which fields are not
// behaviour). The shared core (internal/replay) speaks only the unified format
// and compares what the adapter normalises.
package replay

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	core "github.com/sloprail/harness-mocks/internal/replay"
)

// Adapter is the codex harness's side of a replay (core.Adapter): it reads a
// codex recording into the unified form (Load), turns that into the scenario the
// codex mock takes (Denormalize), runs the mock, and normalises both outputs by
// codex's own rules (Rules).
type Adapter struct{}

// Run replays the recording in runDir: the mock binary is run on the scenario
// generated from it, and what differs from the recording is returned; empty is
// a green replay. A recording the adapter cannot build is an *Unbuildable.
func Run(mock, runDir string) (string, error) { return core.Run(Adapter{}, mock, runDir) }

// Script is the generated scenario for debugging: the main script and each
// sub-agent's.
func Script(runDir string) (string, error) { return core.Script(Adapter{}, runDir) }

// Script is the scenario Denormalize makes of rec, as text.
func (Adapter) Script(rec core.Recording) (string, error) {
	s := Denormalize(rec)
	out := "# main\n" + s.Script
	for name, body := range s.Files {
		if name != "hook.sh" {
			out += "\n# " + name + "\n" + body
		}
	}
	return out, nil
}

// Replay runs the mock on rec's scenario in a hermetic repository, and returns
// the recording's event stream and hook payloads with the mock's, normalised.
func (Adapter) Replay(mock string, rec core.Recording) (want, got core.Observed, err error) {
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
	if err := exec.Command("git", "-C", repo, "init", "-q").Run(); err != nil {
		return want, got, fmt.Errorf("git init: %w", err)
	}
	if err := os.WriteFile(filepath.Join(home, "hooks.json"), []byte(s.HooksJSON), 0o644); err != nil {
		return want, got, err
	}
	for name, body := range s.Files {
		if err := os.WriteFile(filepath.Join(repo, name), []byte(body), 0o755); err != nil {
			return want, got, err
		}
	}
	script := filepath.Join(root, "scenario.sh")
	if err := os.WriteFile(script, []byte(s.Script), 0o755); err != nil {
		return want, got, err
	}

	// hermetic but for the tools (sh, git, jq) the hooks use: no CODEX_* or CLAUDE* variable of ours reaches the mock
	env := []string{"CODEX_HOME=" + home, "TMPDIR=" + tmp, "HOOK_LOG=" + filepath.Join(tmp, "hook.log")}
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "CODEX") && !strings.HasPrefix(kv, "CLAUDE") && !strings.HasPrefix(kv, "TMPDIR=") {
			env = append(env, kv)
		}
	}
	cmd := exec.Command(mock, "exec", "--dangerously-bypass-hook-trust", "--json", "--skip-git-repo-check", "--script", script, "-m", "mock-model", s.Prompt)
	cmd.Dir, cmd.Env = repo, env
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return want, got, &core.MockFailure{Detail: fmt.Sprintf("%v: %s", err, errb.String())}
	}
	hookLog, _ := os.ReadFile(filepath.Join(tmp, "hook.log"))

	// one canonicalisation per side, the event stream first: it names the ids in a fixed order
	rules := Rules(repo, tmp)
	wantC, gotC := core.New(rules), core.New(rules)
	want.Events = wantC.Lines(jsonLines(readFile(filepath.Join(sample, "stream.jsonl"))))
	got.Events = gotC.Lines(jsonLines(out.String()))
	want.Hooks = wantC.Lines(jsonLines(readFile(filepath.Join(sample, "payloads.jsonl"))))
	got.Hooks = gotC.Lines(jsonLines(string(hookLog)))
	// hooks of one event run at the same time, so the order they log in is not the behaviour
	sort.Strings(want.Hooks)
	sort.Strings(got.Hooks)
	return want, got, nil
}
