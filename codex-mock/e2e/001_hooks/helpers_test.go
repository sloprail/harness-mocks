package e2e

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// scenario is one mock run: the hooks it loads, the script that drives it.
type scenario struct {
	// HooksJSON is the user layer's $CODEX_HOME/hooks.json.
	HooksJSON string
	// Files are written into the repository (a hook script, say), mode 0755.
	Files map[string]string
	// Script is the scenario script.
	Script string
	Prompt string
	// Env is added to the mock's environment.
	Env []string
}

// result is what a run left.
type result struct {
	Repo, Home, Tmp string
	Stdout, Stderr  string
	Code            int
}

// execMock runs `exec --json` in a fresh git repository, hermetic but for the
// tools (sh, git, jq) the hooks use: no CODEX_* or CLAUDE* variable of the
// process running the tests reaches it.
func execMock(t *testing.T, s scenario) result {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	r := result{Repo: filepath.Join(root, "repo"), Home: filepath.Join(root, "home", ".codex"), Tmp: filepath.Join(root, "tmp")}
	for _, d := range []string{r.Repo, r.Home, r.Tmp} {
		require.NoError(t, os.MkdirAll(d, 0o755))
	}
	require.NoError(t, exec.Command("git", "-C", r.Repo, "init", "-q").Run())
	if s.HooksJSON != "" {
		require.NoError(t, os.WriteFile(filepath.Join(r.Home, "hooks.json"), []byte(s.HooksJSON), 0o644))
	}
	for name, body := range s.Files {
		require.NoError(t, os.WriteFile(filepath.Join(r.Repo, name), []byte(body), 0o755))
	}
	script := filepath.Join(root, "scenario.sh")
	require.NoError(t, os.WriteFile(script, []byte(s.Script), 0o755))

	env := []string{"CODEX_HOME=" + r.Home, "TMPDIR=" + r.Tmp, "HOOK_LOG=" + filepath.Join(r.Tmp, "hook.log")}
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "CODEX") && !strings.HasPrefix(kv, "CLAUDE") && !strings.HasPrefix(kv, "TMPDIR=") {
			env = append(env, kv)
		}
	}
	cmd := exec.Command(mockBinary, "exec", "--json", "--skip-git-repo-check", "--script", script, "-m", "mock-model", s.Prompt)
	cmd.Dir = r.Repo
	cmd.Env = append(env, s.Env...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		require.ErrorAs(t, err, &exit)
		r.Code = exit.ExitCode()
	}
	r.Stdout, r.Stderr = out.String(), errb.String()
	return r
}

// jsonLines parses text as JSON objects, one per line, skipping other lines.
func jsonLines(text string) []map[string]any {
	var out []map[string]any
	for _, l := range strings.Split(text, "\n") {
		var m map[string]any
		if json.Unmarshal([]byte(strings.TrimSpace(l)), &m) == nil && m != nil {
			out = append(out, m)
		}
	}
	return out
}

// hookLog is what the run's hooks wrote to $HOOK_LOG, as objects.
func (r result) hookLog() []map[string]any {
	b, _ := os.ReadFile(filepath.Join(r.Tmp, "hook.log"))
	return jsonLines(string(b))
}

// stream is the event stream the run printed.
func (r result) stream() []map[string]any { return jsonLines(r.Stdout) }

// commands are the shell commands the stream shows as run to completion, in
// order, with their exit codes.
func (r result) commands() (cmds []string, exits []float64) {
	for _, e := range r.stream() {
		item, _ := e["item"].(map[string]any)
		if e["type"] == "item.completed" && item["type"] == "command_execution" {
			cmds = append(cmds, innerCommand(item["command"].(string)))
			exits = append(exits, item["exit_code"].(float64))
		}
	}
	return
}

// innerCommand is the command inside `/bin/<shell> -c[l] '<command>'`.
func innerCommand(shellLine string) string {
	i := strings.Index(shellLine, "'")
	return strings.ReplaceAll(strings.TrimSuffix(shellLine[i+1:], "'"), `'\''`, "'")
}

// rollout is the session file the run wrote.
func (r result) rollout(t *testing.T) string {
	t.Helper()
	var text string
	require.NoError(t, filepath.Walk(filepath.Join(r.Home, "sessions"), func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			b, _ := os.ReadFile(p)
			text = string(b)
		}
		return nil
	}))
	return text
}

// hooksJSON is a hooks.json with one command handler per event, all running
// the same command (the matcher is any).
func hooksJSON(command string, events ...string) string {
	h := map[string]any{}
	for _, ev := range events {
		h[ev] = []any{map[string]any{"hooks": []any{map[string]any{"type": "command", "command": command}}}}
	}
	b, _ := json.Marshal(map[string]any{"hooks": h})
	return string(b)
}

const (
	// callThenResult makes one Bash call per run of the script, taken from
	// $CALLS (one command per line), and then a final message and a result.
	callThenResult = `#!/bin/sh
n=$(grep -c function_call_output "$A10N_MOCK_SESSION_FILE")
cmd=$(sed -n "$((n+1))p" "$CALLS")
if [ -n "$cmd" ]; then
  printf '{"type":"assistant","message":{"content":[{"type":"tool_use","id":"call_%s","name":"Bash","input":{"command":%s}}]}}\n' "$n" "$(printf '%s' "$cmd" | jq -Rs .)"
  exit 0
fi
printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"text","text":"DONE"}]}}' '{"type":"result","subtype":"success","result":"DONE"}'
`
)

// withCalls is env for callThenResult: the commands it runs, in order.
func withCalls(t *testing.T, cmds ...string) []string {
	t.Helper()
	f := filepath.Join(t.TempDir(), "calls")
	require.NoError(t, os.WriteFile(f, []byte(strings.Join(cmds, "\n")+"\n"), 0o644))
	return []string{"CALLS=" + f}
}
