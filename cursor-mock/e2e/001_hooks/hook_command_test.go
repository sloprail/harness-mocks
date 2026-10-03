package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// hookRun is a run of the mock whose project is a fresh workspace with the
// given hooks.json and one command for the agent to run, started from the
// directory startDir names (given the workspace) with the extra arguments.
type hookRun struct{ ws, log string }

func startHookRun(t *testing.T, hooksJSON func(ws string) string, startDir func(ws string) string, extra func(ws string) []string) hookRun {
	t.Helper()
	ws, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	scratch := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(ws, ".cursor", "hooks"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(ws, ".cursor", "hooks.json"), []byte(hooksJSON(ws)), 0o644))
	call := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"tu_0","name":"Bash","input":{"command":"echo hi"}}]}}`
	require.NoError(t, os.WriteFile(filepath.Join(scratch, "0.json"), []byte(call+"\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(scratch, "end.json"), []byte(`{"type":"result","subtype":"success","result":"DONE"}`+"\n"), 0o644))
	script := filepath.Join(scratch, "scenario.sh")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\nn=$(grep -c '\"type\":\"tool_use\"' \"$A10N_MOCK_SESSION_FILE\" 2>/dev/null)\nn=${n:-0}\nf=\""+scratch+"/$n.json\"\n[ -f \"$f\" ] || f=\""+scratch+"/end.json\"\ncat \"$f\"\n"), 0o755))
	logPath := filepath.Join(scratch, "log.txt")
	args := []string{"-p", "--force", "--output-format", "stream-json", "--script", script}
	if extra != nil {
		args = append(args, extra(ws)...)
	}
	cmd := exec.Command(binary, append(args, "go")...)
	start := startDir(ws)
	cmd.Dir, cmd.Env = start, []string{"PATH=" + os.Getenv("PATH"), "HOME=" + t.TempDir(), "HOOK_LOG=" + logPath, "PWD=" + start}
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	return hookRun{ws: ws, log: logPath}
}

func (r hookRun) read(t *testing.T, suffix string) string {
	t.Helper()
	b, err := os.ReadFile(r.log + suffix)
	require.NoError(t, err, suffix)
	return strings.TrimSpace(string(b))
}

// TestAHookCommandGivenAsAnAbsolutePathRunsFromTheProjectRoot: the docs say a
// project hook's command can be a shell string, an absolute path or a relative
// path, and runs from the project root: a command that is the absolute path of
// a script outside the project (given an argument, as a shell line may be)
// runs, reads its event's payload as JSON on stdin and runs from the project
// root, as the relative path of a script in it does.
// sr:proves hook-command-handler/cursor
func TestAHookCommandGivenAsAnAbsolutePathRunsFromTheProjectRoot(t *testing.T) {
	outside := t.TempDir()
	script := filepath.Join(outside, "abs.sh")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\ncat > \"$HOOK_LOG.in\"\npwd -P > \"$HOOK_LOG.pwd\"\necho \"$1\" > \"$HOOK_LOG.arg\"\n"), 0o755))

	r := startHookRun(t,
		func(string) string {
			return `{"version":1,"hooks":{"afterShellExecution":[{"command":` + jsonString(script+" one") + `}]}}`
		},
		func(ws string) string { return ws }, nil)
	require.Equal(t, r.ws, r.read(t, ".pwd"), "an absolute-path hook runs from the project root, not from where its script lives")
	require.Equal(t, "one", r.read(t, ".arg"), "its argument reaches it")
	in := r.read(t, ".in")
	require.Contains(t, in, `"hook_event_name":"afterShellExecution"`, "the payload is on its stdin")
	require.Contains(t, in, `"command":"echo hi"`)
}

// TestProjectHooksRunFromTheProjectRootWhereverTheProcessStarted: recorded
// (runs/symlinked-cwd: cursor-agent started from a symlink to the workspace),
// a hook runs in the resolved project root, its logical and physical working
// directory alike, never in the symlink the process was started from. The
// mock's hooks do the same when it is started from a symlink to the workspace,
// and when it is started elsewhere and told the workspace (--workspace).
// sr:proves hook-command-handler/cursor
func TestProjectHooksRunFromTheProjectRootWhereverTheProcessStarted(t *testing.T) {
	dir := newestSample(t, "symlinked-cwd")
	var recorded int
	for _, m := range readJSONL(t, filepath.Join(dir, "payloads.jsonl")) {
		if r, ok := m["hook_result"].(map[string]any); ok && r["event"] != "afterAgentThought" {
			recorded++
			require.Equal(t, "<RUN>", r["pwd"], "recorded: the logical working directory of the "+r["event"].(string)+" hook is the project root")
			require.Equal(t, "<RUN>", r["pwd_physical"])
		}
	}
	require.NotZero(t, recorded)

	hooks := func(string) string {
		return `{"version":1,"hooks":{"afterShellExecution":[{"command":"pwd > \"$HOOK_LOG.pwd\"; pwd -P > \"$HOOK_LOG.physical\"; cat >/dev/null"}]}}`
	}
	var link string
	r := startHookRun(t, hooks, func(ws string) string {
		link = filepath.Join(t.TempDir(), "link")
		require.NoError(t, os.Symlink(ws, link))
		return link
	}, nil)
	require.Equal(t, r.ws, r.read(t, ".pwd"), "started from a symlink: the project root, not the symlink "+link)
	require.Equal(t, r.ws, r.read(t, ".physical"))

	// started elsewhere, the workspace is the one it is told
	r = startHookRun(t, hooks, func(string) string { return t.TempDir() }, func(ws string) []string { return []string{"--workspace", ws} })
	require.Equal(t, r.ws, r.read(t, ".pwd"), "started elsewhere with --workspace: the project root")
	require.Equal(t, r.ws, r.read(t, ".physical"))
}
