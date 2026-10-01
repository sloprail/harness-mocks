// Package childenv is Claude Code's names for the harness and the session in
// the environment of a process it starts (a hook, a Bash tool command).
package childenv

import (
	"os"
	"strconv"
)

// Identity is what every child process of a Claude Code session sees:
// CLAUDECODE=1 and CLAUDE_CODE_ENTRYPOINT mark "running under Claude Code",
// CLAUDE_CODE_CHILD_SESSION=1 that Claude Code itself launched the process,
// CLAUDE_PID the harness's own pid, CLAUDE_CODE_SESSION_ATTENDED=0 that no one
// attends a print-mode session, and CLAUDE_CODE_SESSION_ID names the active
// session (left out when unknown).
// The entrypoint is sdk-cli, what the recorded `claude -p` run shows
// (snapshots/runs/subprocess-session-env); the docs do not name its values.
//
// sr:provides subprocess-session-env/claude
// sr:docs https://code.claude.com/docs/en/env-vars (CLAUDECODE, CLAUDE_CODE_CHILD_SESSION, CLAUDE_CODE_SESSION_ID)
func Identity(sessionID string) map[string]string {
	return map[string]string{
		"CLAUDECODE":                   "1",
		"CLAUDE_CODE_ENTRYPOINT":       "sdk-cli",
		"CLAUDE_CODE_CHILD_SESSION":    "1",
		"CLAUDE_CODE_SESSION_ATTENDED": "0",
		"CLAUDE_PID":                   strconv.Itoa(os.Getpid()),
		"CLAUDE_CODE_SESSION_ID":       sessionID,
	}
}
