// Package childenv is Codex's names for the harness and the session in the
// environment of a process it starts.
package childenv

import "github.com/sloprail/harness-mocks/internal/procexec"

// Version is the codex release whose behaviour the mock reproduces
// (snapshots/MANIFEST.yaml).
const Version = "0.159.3"

// ToolIdentity is what a shell command the agent runs sees, whatever it
// inherited (recorded: runs/nested-session-env, launched over decoys):
// CODEX_THREAD_ID and CODEX_SESSION_ID name the active session and
// CODEX_VERSION the harness release.
//
// sr:provides subprocess-session-env/codex
// sr:docs https://developers.openai.com/codex/hooks#common-input-fields
func ToolIdentity(sessionID string) map[string]string {
	return map[string]string{
		"CODEX_THREAD_ID":  sessionID,
		"CODEX_SESSION_ID": sessionID,
		"CODEX_VERSION":    Version,
	}
}

// ToolDefaults is what a shell command sees only when none was inherited:
// CODEX_CI=1, which an inherited value passes through over (recorded: a decoy
// CODEX_CI reached the command unchanged).
func ToolDefaults() map[string]string { return map[string]string{"CODEX_CI": "1"} }

// HookIdentity is what a hook command sees beyond what it inherited: nothing.
// Codex hands a hook no session variable; the session is in its payload
// (recorded: runs/subprocess-session-env, whose hooks saw only CODEX_HOME and
// the package manager's variables, and runs/nested-session-env, where an
// inherited CODEX_THREAD_ID reached the hook unchanged).
func HookIdentity() map[string]string { return nil }

// ToolEnv is the whole environment of a shell command: the mock's own,
// with this run's identity.
func ToolEnv(inherited []string, sessionID string) []string {
	return procexec.Env(inherited, ToolIdentity(sessionID), ToolDefaults())
}
