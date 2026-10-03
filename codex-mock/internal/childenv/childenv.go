// Package childenv is Codex's names for the harness and the session in the
// environment of a process it starts.
package childenv

import (
	"os"
	"path/filepath"

	"github.com/sloprail/harness-mocks/internal/procexec"
)

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
	id := Launcher()
	id["CODEX_THREAD_ID"] = sessionID
	id["CODEX_SESSION_ID"] = sessionID
	id["CODEX_VERSION"] = Version
	return id
}

// Launcher is how the harness was started, which every child it spawns, tool
// command or hook, carries and which overrides an inherited value (recorded:
// runs/nested-session-env, where a decoy CODEX_MANAGED_BY_NPM reached neither
// the command nor the hooks): the npm launcher's CODEX_MANAGED_BY_NPM=1 and
// CODEX_MANAGED_PACKAGE_ROOT, the directory the package is installed in. The
// mock is no npm package; its root is the directory holding its executable.
func Launcher() map[string]string {
	root := ""
	if exe, err := os.Executable(); err == nil {
		root = filepath.Dir(exe)
	}
	return map[string]string{"CODEX_MANAGED_BY_NPM": "1", "CODEX_MANAGED_PACKAGE_ROOT": root}
}

// ToolDefaults is what a shell command sees only when none was inherited:
// CODEX_CI=1, which an inherited value passes through over (recorded: a decoy
// CODEX_CI reached the command unchanged).
func ToolDefaults() map[string]string { return map[string]string{"CODEX_CI": "1"} }

// HookIdentity is what a hook command sees beyond what it inherited: how the
// harness was started (Launcher), and no session variable. Codex hands a hook
// none; the session is in its payload
// (recorded: runs/subprocess-session-env, whose hooks saw only CODEX_HOME and
// the package manager's variables, and runs/nested-session-env, where an
// inherited CODEX_THREAD_ID reached the hook unchanged).
func HookIdentity() map[string]string { return Launcher() }

// ToolEnv is the whole environment of a shell command: the mock's own,
// with this run's identity.
func ToolEnv(inherited []string, sessionID string) []string {
	return procexec.Env(inherited, ToolIdentity(sessionID), ToolDefaults())
}
