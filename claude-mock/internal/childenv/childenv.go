// Package childenv is Claude Code's names for the harness and the session in
// the environment of a process it starts (a hook, a Bash tool command).
package childenv

// Identity is what every child process of a Claude Code session sees:
// CLAUDECODE=1 and CLAUDE_CODE_ENTRYPOINT mark "running under Claude Code", and
// CLAUDE_CODE_SESSION_ID names the active session (left out when unknown).
//
// sr:provides subprocess-session-env/claude
// sr:docs https://code.claude.com/docs/en/env-vars (CLAUDECODE, CLAUDE_CODE_ENTRYPOINT, CLAUDE_CODE_SESSION_ID)
func Identity(sessionID string) map[string]string {
	return map[string]string{
		"CLAUDECODE":             "1",
		"CLAUDE_CODE_ENTRYPOINT": "cli",
		"CLAUDE_CODE_SESSION_ID": sessionID,
	}
}
