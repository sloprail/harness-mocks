package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// With the A10N_CURSOR_MOCK_STOP opt-in a plugin's hooks are those of the TUI recordings
// (runs/tui-plugins, runs/tui-plugins-event-gating): a plugin's stop hook never fires, though the
// project's does, and its afterAgentResponse hook fires only when the project's hooks.json has one
// for the event too. Without the opt-in print mode fires neither event at all.

// printPluginRun runs the mock in print mode with a --plugin-dir plugin whose stop and
// afterAgentResponse hooks log "plugin <event>", beside a project whose hooks.json holds the given
// events' hooks logging "project <event>"; it returns what was logged.
func printPluginRun(t *testing.T, optIn bool, projectEvents ...string) []string {
	t.Helper()
	ws, scratch, home := shortTempDir(t), t.TempDir(), t.TempDir()
	log := filepath.Join(scratch, "log")
	hook := func(who, event string) string {
		return `{"command":"cat >/dev/null; echo ` + who + ` ` + event + ` >> ` + log + `"}`
	}
	file := func(who string, events ...string) string {
		var parts []string
		for _, e := range events {
			parts = append(parts, `"`+e+`":[`+hook(who, e)+`]`)
		}
		return `{"version":1,"hooks":{` + strings.Join(parts, ",") + `}}`
	}
	plugin := filepath.Join(ws, "plug")
	require.NoError(t, os.MkdirAll(filepath.Join(plugin, ".cursor-plugin"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(plugin, "hooks"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(plugin, ".cursor-plugin", "plugin.json"), []byte(`{"name":"plug"}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(plugin, "hooks", "hooks.json"), []byte(file("plugin", "stop", "afterAgentResponse")), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(ws, ".cursor"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(ws, ".cursor", "hooks.json"), []byte(file("project", projectEvents...)), 0o644))
	script := filepath.Join(scratch, "scenario.sh")
	require.NoError(t, os.WriteFile(script, []byte(`#!/bin/sh
printf '{"type":"assistant","message":{"content":[{"type":"text","text":"DONE"}]}}\n{"type":"result","subtype":"success","is_error":false,"result":"DONE"}\n'
`), 0o755))
	cmd := exec.Command(binary, "-p", "--force", "--trust", "--model", "auto", "--output-format", "stream-json", "--plugin-dir", "plug", "go")
	cmd.Dir = ws
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + home, "TMPDIR=" + scratch, "A10N_MOCK_SCRIPT=" + script}
	if optIn {
		cmd.Env = append(cmd.Env, "A10N_CURSOR_MOCK_STOP=1")
	}
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	raw, _ := os.ReadFile(log)
	if len(raw) == 0 {
		return nil
	}
	return strings.Split(strings.TrimSpace(string(raw)), "\n")
}

func TestPrintStopOptInPluginStopNeverFiresProjectStopDoes(t *testing.T) {
	got := printPluginRun(t, true, "stop", "afterAgentResponse")
	require.ElementsMatch(t, []string{"project afterAgentResponse", "plugin afterAgentResponse", "project stop"}, got)
}

func TestPrintStopOptInPluginResponseHookNeedsTheProjectsToo(t *testing.T) {
	require.Equal(t, []string{"project stop"}, printPluginRun(t, true, "stop"))
}

func TestPrintWithoutOptInFiresNeitherStopNorResponse(t *testing.T) {
	require.Empty(t, printPluginRun(t, false, "stop", "afterAgentResponse"))
}
