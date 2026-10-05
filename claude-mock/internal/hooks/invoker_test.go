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
// sr:proves hook-command-handler/claude
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
// sr:proves hook-command-handler/claude
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

// Every command hook sees this run's identity even when the mock runs inside
// another session (recorded: runs/nested-session-env, claude launched over
// decoys): CLAUDECODE=1, CLAUDE_CODE_CHILD_SESSION=1, CLAUDE_CODE_SESSION_ATTENDED=0,
// the harness's pid as CLAUDE_PID and the active CLAUDE_CODE_SESSION_ID replace
// whatever was inherited, while an inherited CLAUDE_CODE_ENTRYPOINT (the
// launcher's) passes through. A tool that detects "am I under a harness" keys off
// these (sr-agent refuses with ErrNoHarness without them).
//
// This exercises the ONE seam every hook passes through — invokeCommand — so it
// covers the root Stop/PreToolUse hooks AND the sub-agent SubagentStart/SubagentStop
// hooks (agent.go fires those through this same invoker) AND the hooks a nested
// sub-agent run fires (runSubagent constructs an Invoker the same way). All three
// paths reach the hook env through this method, so proving it here proves it for all.
// sr:proves subprocess-session-env/claude
func TestInvokeCommand_SetsClaudeCodeEnvOnHook(t *testing.T) {
	for k, v := range map[string]string{"CLAUDECODE": "decoy", "CLAUDE_CODE_ENTRYPOINT": "decoy-launcher",
		"CLAUDE_CODE_SESSION_ID": "decoy-outer-session", "CLAUDE_CODE_CHILD_SESSION": "decoy",
		"CLAUDE_CODE_SESSION_ATTENDED": "decoy", "CLAUDE_PID": "decoy"} {
		t.Setenv(k, v)
	}
	dir := t.TempDir()
	envFile := filepath.Join(dir, "hookenv.txt")
	// The hook records the six variables it received, one per line, so the test
	// reads back exactly what reached the hook subprocess's environment.
	script := writeExecScript(t, dir, "env.sh",
		"#!/bin/sh\ncat >/dev/null\n"+
			"{ echo \"CLAUDECODE=$CLAUDECODE\"; "+
			"echo \"CLAUDE_CODE_ENTRYPOINT=$CLAUDE_CODE_ENTRYPOINT\"; "+
			"echo \"CLAUDE_CODE_SESSION_ID=$CLAUDE_CODE_SESSION_ID\"; "+
			"echo \"CLAUDE_CODE_CHILD_SESSION=$CLAUDE_CODE_CHILD_SESSION\"; "+
			"echo \"CLAUDE_CODE_SESSION_ATTENDED=$CLAUDE_CODE_SESSION_ATTENDED\"; "+
			"echo \"CLAUDE_PID=$CLAUDE_PID\"; } > \""+envFile+"\"\n")

	settings := &Settings{Hooks: map[EventName][]HookEntry{
		EventStop: {{Matcher: "*", Hooks: []HandlerSpec{
			{Type: "command", Command: `"` + script + `"`},
		}}},
	}}
	inv := NewInvoker(settings, dir, "sess-xyz")

	_, err := inv.Fire(context.Background(), Input{HookEventName: EventStop})
	require.NoError(t, err)

	data, readErr := os.ReadFile(envFile)
	require.NoError(t, readErr, "hook never recorded its environment")
	got := string(data)
	assert.Contains(t, got, "CLAUDECODE=1\n", "CLAUDECODE is this run's")
	assert.Contains(t, got, "CLAUDE_CODE_ENTRYPOINT=decoy-launcher\n", "the launcher's entrypoint passes through")
	assert.Contains(t, got, "CLAUDE_CODE_SESSION_ID=sess-xyz\n", "CLAUDE_CODE_SESSION_ID is this run's")
	assert.Contains(t, got, "CLAUDE_CODE_CHILD_SESSION=1\n", "CLAUDE_CODE_CHILD_SESSION is this run's")
	assert.Contains(t, got, "CLAUDE_CODE_SESSION_ATTENDED=0\n", "a print-mode session is unattended")
	assert.Contains(t, got, "CLAUDE_PID="+strconv.Itoa(os.Getpid())+"\n", "CLAUDE_PID is the harness's own pid")
}

// CLAUDECODE=1 and CLAUDE_CODE_ENTRYPOINT are set even when the invoker has NO
// session id (the empty-sessionID construction a session-less run uses), and
// with no entrypoint inherited it is sdk-cli (runs/subprocess-session-env). An
// inherited session id is not passed on in its place: it names another session.
// sr:proves subprocess-session-env/claude
func TestInvokeCommand_SetsHarnessEnvWithoutSessionID(t *testing.T) {
	t.Setenv("CLAUDECODE", "decoy")
	t.Setenv("CLAUDE_CODE_ENTRYPOINT", "")
	t.Setenv("CLAUDE_CODE_SESSION_ID", "decoy-outer-session")
	dir := t.TempDir()
	envFile := filepath.Join(dir, "hookenv.txt")
	script := writeExecScript(t, dir, "env.sh",
		"#!/bin/sh\ncat >/dev/null\n"+
			"{ echo \"CLAUDECODE=$CLAUDECODE\"; "+
			"echo \"CLAUDE_CODE_ENTRYPOINT=$CLAUDE_CODE_ENTRYPOINT\"; "+
			"echo \"CLAUDE_CODE_SESSION_ID=$CLAUDE_CODE_SESSION_ID\"; } > \""+envFile+"\"\n")

	settings := &Settings{Hooks: map[EventName][]HookEntry{
		EventStop: {{Matcher: "*", Hooks: []HandlerSpec{
			{Type: "command", Command: `"` + script + `"`},
		}}},
	}}
	inv := NewInvoker(settings, dir, "") // no session id

	_, err := inv.Fire(context.Background(), Input{HookEventName: EventStop})
	require.NoError(t, err)

	data, readErr := os.ReadFile(envFile)
	require.NoError(t, readErr)
	got := string(data)
	assert.Contains(t, got, "CLAUDECODE=1\n")
	assert.Contains(t, got, "CLAUDE_CODE_ENTRYPOINT=sdk-cli\n")
	assert.Contains(t, got, "CLAUDE_CODE_SESSION_ID=\n", "the outer session's id must not reach the hook")
}

// Exit 2 from a command hook is a blocking error surfaced via Fire's error.
// sr:proves hook-exit-code-semantics/claude
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
// sr:proves hook-timeout/claude
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
// sr:proves hook-timeout/claude
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

// A command hook runs in a session of its own, with no controlling terminal, so
// neither it nor its children can open /dev/tty (docs, Hook input and output).
// The hook's shell is the leader of its session (the session id is the pid its
// child sees as its parent) and an attempt to open the terminal fails.
// sr:proves hook-command-handler/claude
func TestInvokeCommand_HookHasNoControllingTerminal(t *testing.T) {
	_, err := exec.LookPath("python3") // it reports the hook's session id
	require.NoError(t, err, "python3 is not installed: the test needs it (adr/tests-fail-on-missing-tool)")
	dir := t.TempDir()
	out := filepath.Join(dir, "out.txt")
	hook := "cat >/dev/null; python3 -c 'import os; print(os.getsid(0)==os.getppid())' > " + out +
		"; ( : </dev/tty ) 2>/dev/null && echo tty-opened >> " + out + " || echo no-tty >> " + out
	settings := &Settings{Hooks: map[EventName][]HookEntry{
		EventStop: {{Matcher: "*", Hooks: []HandlerSpec{{Type: "command", Command: hook, Timeout: 30}}}},
	}}
	_, err = NewInvoker(settings, dir, "sid").Fire(context.Background(), Input{HookEventName: EventStop})
	require.NoError(t, err)
	got, err := os.ReadFile(out)
	require.NoError(t, err)
	assert.Equal(t, "True\nno-tty\n", string(got), "the hook leads its own session and cannot open /dev/tty")
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

// TestFire_RunsEveryHandlerAndReturnsTheFirstBlock: real Claude Code runs all
// of an event's matching hooks, so one that exits 2 does not stop the others;
// the block comes back as a *BlockError quoting "[<command>]: <stderr>".
// sr:proves hooks-all-matching-run/claude
func TestFire_RunsEveryHandlerAndReturnsTheFirstBlock(t *testing.T) {
	dir := t.TempDir()
	ran := filepath.Join(dir, "ran")
	block := writeExecScript(t, dir, "block.sh", "#!/bin/sh\necho refused >&2\nexit 2\n")
	after := writeExecScript(t, dir, "after.sh", "#!/bin/sh\ntouch "+ran+"\n")
	s := &Settings{Hooks: map[EventName][]HookEntry{EventStop: {{Hooks: []HandlerSpec{
		{Type: "command", Command: block}, {Type: "command", Command: after},
	}}}}}
	var recorded []HandlerRun
	inv := NewInvoker(s, dir, "sid")
	inv.SetRecorder(func(_ Input, runs []HandlerRun) { recorded = runs })
	_, err := inv.Fire(context.Background(), Input{HookEventName: EventStop, Cwd: dir})
	var be *BlockError
	require.ErrorAs(t, err, &be)
	assert.Equal(t, "["+block+"]: refused\n", be.Quoted())
	_, statErr := os.Stat(ran)
	assert.NoError(t, statErr, "the second handler ran too")
	require.Len(t, recorded, 2)
	assert.True(t, recorded[0].Blocked)
	assert.Equal(t, "[h]: No stderr output", QuoteBlock("h", ""))
}
