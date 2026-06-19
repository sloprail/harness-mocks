package hooks

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeExecScript writes an executable sh script to dir/name and returns its path.
func writeExecScript(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(p, []byte(body), 0o755))
	return p
}

// invokeCommand runs command hooks through /bin/sh -c, so a quoted script path
// (the form plugin hooks.json emits) executes. A strings.Fields split would keep
// the quotes in argv[0] and fail to find the binary.
func TestInvokeCommand_QuotedPathRunsViaShell(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "ran.txt")
	script := writeExecScript(t, dir, "hook.sh", "#!/bin/sh\ncat >/dev/null\necho ran > \""+marker+"\"\n")

	settings := &Settings{Hooks: map[EventName][]HookEntry{
		EventStop: {{Matcher: "*", Hooks: []HandlerSpec{
			{Type: "command", Command: `"` + script + `"`}, // quoted, like a plugin
		}}},
	}}
	inv := NewInvoker(settings, dir)

	_, err := inv.Fire(context.Background(), Input{HookEventName: EventStop})
	require.NoError(t, err)

	_, statErr := os.Stat(marker)
	require.NoError(t, statErr, "quoted-path hook must execute via the shell")
}

// A command carrying arguments and an env-var reference runs as a shell line.
func TestInvokeCommand_ArgsAndEnvRunViaShell(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "out.txt")
	script := writeExecScript(t, dir, "hook.sh",
		"#!/bin/sh\ncat >/dev/null\necho \"$1 $MY_VAR\" > \""+marker+"\"\n")
	t.Setenv("MY_VAR", "fromenv")

	settings := &Settings{Hooks: map[EventName][]HookEntry{
		EventStop: {{Matcher: "*", Hooks: []HandlerSpec{
			{Type: "command", Command: `"` + script + `" arg1`},
		}}},
	}}
	inv := NewInvoker(settings, dir)

	_, err := inv.Fire(context.Background(), Input{HookEventName: EventStop})
	require.NoError(t, err)

	data, readErr := os.ReadFile(marker)
	require.NoError(t, readErr)
	assert.Equal(t, "arg1 fromenv\n", string(data))
}

// Exit 2 from a command hook is a blocking error surfaced via Fire's error.
func TestInvokeCommand_Exit2Blocks(t *testing.T) {
	dir := t.TempDir()
	script := writeExecScript(t, dir, "block.sh", "#!/bin/sh\ncat >/dev/null\necho 'denied' >&2\nexit 2\n")

	settings := &Settings{Hooks: map[EventName][]HookEntry{
		EventPreToolUse: {{Matcher: "Agent", Hooks: []HandlerSpec{
			{Type: "command", Command: `"` + script + `"`},
		}}},
	}}
	inv := NewInvoker(settings, dir)

	_, err := inv.Fire(context.Background(), Input{HookEventName: EventPreToolUse, ToolName: "Agent"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "denied")
}
