package hooks

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

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
	inv := NewInvoker(settings, dir, "test-session")

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
	inv := NewInvoker(settings, dir, "test-session")

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
	inv := NewInvoker(settings, dir, "test-session")

	_, err := inv.Fire(context.Background(), Input{HookEventName: EventPreToolUse, ToolName: "Agent"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "denied")
}

// alive reports whether pid names a live process. Signal 0 performs the
// permission-and-existence check without delivering anything.
func alive(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return p.Signal(syscall.Signal(0)) == nil
}

// The declared timeout must actually bound the call.
//
// It did not. invokeCommand runs the hook via `/bin/sh -c`, and on expiry
// context cancellation killed only the SHELL. A `sleep` the shell spawned is
// the shell's own child: it survived, kept the inherited stdout/stderr pipes
// open, and cmd.Run() blocks until every writer to those pipes is gone — so the
// call returned only when the grandchild finished on its own. A hook declaring
// `"timeout": 1` around a 20s sleep took the full 20s.
func TestInvokeCommand_TimeoutBoundsTheCall(t *testing.T) {
	dir := t.TempDir()
	settings := &Settings{Hooks: map[EventName][]HookEntry{
		EventStop: {{Matcher: "*", Hooks: []HandlerSpec{
			{Type: "command", Command: "sleep 20", Timeout: 1},
		}}},
	}}
	inv := NewInvoker(settings, dir, "test-session")

	start := time.Now()
	_, err := inv.Fire(context.Background(), Input{HookEventName: EventStop})
	elapsed := time.Since(start)

	require.NoError(t, err, "an expired hook is not a blocking (exit 2) refusal")
	assert.Less(t, elapsed, 10*time.Second,
		"a hook declaring timeout:1 took %s — the deadline did not bound the call", elapsed)
}

// The kill must reach the grandchild, not merely the shell.
//
// This is the half that a cmd.Cancel/WaitDelay fix would leave broken: Go would
// stop WAITING on the pipes, the call would return on time, and the runaway
// process would still be running — now orphaned, and invisible to the harness
// that spawned it. The hook here publishes its grandchild's pid so the test can
// go looking for it afterwards.
func TestInvokeCommand_TimeoutKillsGrandchild(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "grandchild.pid")

	// `sleep` is spawned as a background child of the shell, so the shell is
	// NOT the process holding the pipes open — exactly the production shape of
	// a hook that shells out to something longer-lived.
	settings := &Settings{Hooks: map[EventName][]HookEntry{
		EventStop: {{Matcher: "*", Hooks: []HandlerSpec{
			{Type: "command", Command: "sleep 20 & echo $! > " + pidFile + "; wait", Timeout: 1},
		}}},
	}}
	inv := NewInvoker(settings, dir, "test-session")

	start := time.Now()
	_, err := inv.Fire(context.Background(), Input{HookEventName: EventStop})
	elapsed := time.Since(start)
	require.NoError(t, err)
	require.Less(t, elapsed, 10*time.Second, "the deadline did not bound the call")

	raw, readErr := os.ReadFile(pidFile)
	require.NoError(t, readErr, "hook never recorded its grandchild pid")
	pid, convErr := strconv.Atoi(strings.TrimSpace(string(raw)))
	require.NoError(t, convErr)

	// Give the signal a moment to be reaped before concluding it survived.
	deadline := time.Now().Add(2 * time.Second)
	for alive(pid) && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	assert.False(t, alive(pid),
		"grandchild pid %d outlived the expired hook: the kill reached the shell but not its children", pid)
}

// A hook finishing inside its deadline is untouched, and its output is read.
//
// The guard against "fix" by killing everything early: a process group that is
// signalled unconditionally, or a deadline applied to the wrong clock, passes
// both tests above and breaks every hook that works.
func TestInvokeCommand_FastHookNotKilled(t *testing.T) {
	dir := t.TempDir()
	settings := &Settings{Hooks: map[EventName][]HookEntry{
		EventStop: {{Matcher: "*", Hooks: []HandlerSpec{
			{Type: "command", Command: `cat >/dev/null; echo '{"systemMessage":"finished"}'`, Timeout: 30},
		}}},
	}}
	inv := NewInvoker(settings, dir, "test-session")

	out, err := inv.Fire(context.Background(), Input{HookEventName: EventStop})
	require.NoError(t, err)
	assert.Equal(t, "finished", out.SystemMessage,
		"a hook well inside its deadline must run to completion and be read")
}

// Setpgid puts the hook in its own process group. Were it left in the mock's
// group, the negative-pid kill would signal the mock itself — the test suite
// would die rather than fail, so assert the separation directly.
func TestInvokeCommand_HookRunsInItsOwnProcessGroup(t *testing.T) {
	dir := t.TempDir()
	pgidFile := filepath.Join(dir, "pgid.txt")

	settings := &Settings{Hooks: map[EventName][]HookEntry{
		EventStop: {{Matcher: "*", Hooks: []HandlerSpec{
			{Type: "command", Command: "cat >/dev/null; ps -o pgid= -p $$ > " + pgidFile, Timeout: 30},
		}}},
	}}
	inv := NewInvoker(settings, dir, "test-session")

	_, err := inv.Fire(context.Background(), Input{HookEventName: EventStop})
	require.NoError(t, err)

	raw, readErr := os.ReadFile(pgidFile)
	require.NoError(t, readErr)
	hookPGID, convErr := strconv.Atoi(strings.TrimSpace(string(raw)))
	require.NoError(t, convErr)

	ownPGID, sysErr := syscall.Getpgid(os.Getpid())
	require.NoError(t, sysErr)
	assert.NotEqual(t, ownPGID, hookPGID,
		"hook shares the mock's process group: killing -pgid would signal the mock itself")
}

// Guards the assumption the fix rests on: the bug is real and is about the
// PIPES, not about the shell surviving. The shell is killed on time even today;
// what overruns is the WAIT. If a future Go release makes CommandContext close
// the pipes itself, this test starts failing and the extra machinery can go.
func TestExecCommandContext_LeaksGrandchildWithoutProcessGroup(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "pid")

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", "sleep 5 & echo $! > "+pidFile+"; wait")
	var out strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &out

	start := time.Now()
	_ = cmd.Run()
	elapsed := time.Since(start)

	assert.Greater(t, elapsed, 3*time.Second,
		"plain CommandContext no longer blocks on the grandchild's pipes — the workaround may be removable")
}
