// Package childenv is Claude Code's names for the harness and the session in
// the environment of a process it starts (a hook, a Bash tool command).
package childenv

import (
	"os"
	"strconv"
)

// Identity is what every child process of a Claude Code session sees, whatever
// it inherited (recorded: runs/nested-session-env, launched over decoys):
// CLAUDECODE=1 marks "running under Claude Code", CLAUDE_CODE_CHILD_SESSION=1
// that Claude Code itself launched the process, CLAUDE_PID the harness's own
// pid, CLAUDE_CODE_SESSION_ATTENDED=0 that no one attends a print-mode session,
// and CLAUDE_CODE_SESSION_ID names the active session (left out when unknown).
//
// sr:provides subprocess-session-env/claude
// sr:docs https://code.claude.com/docs/en/env-vars (CLAUDECODE, CLAUDE_CODE_CHILD_SESSION, CLAUDE_CODE_SESSION_ID, CLAUDE_PID)
func Identity(sessionID string) map[string]string {
	return map[string]string{
		"CLAUDECODE":                   "1",
		"CLAUDE_CODE_CHILD_SESSION":    "1",
		"CLAUDE_CODE_SESSION_ATTENDED": "0",
		"CLAUDE_PID":                   strconv.Itoa(os.Getpid()),
		"CLAUDE_CODE_SESSION_ID":       sessionID,
	}
}

// Defaults are what a child sees only when Claude Code inherited none:
// CLAUDE_CODE_ENTRYPOINT is how the harness was started, which its launcher
// declares (an inherited value passes through, recorded in
// runs/nested-session-env); a print-mode run launched bare says sdk-cli
// (runs/subprocess-session-env). The docs do not name its values.
func Defaults() map[string]string {
	return map[string]string{"CLAUDE_CODE_ENTRYPOINT": "sdk-cli"}
}
