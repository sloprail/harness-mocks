// Package childenv is Cursor's names for the harness and the session in the
// environment of a process it starts (a Shell tool command).
package childenv

import (
	"os/user"
	"path/filepath"
)

// Identity is what a Shell tool command sees whatever it inherited (recorded:
// runs/nested-session-env, launched over decoys): CURSOR_CONVERSATION_ID names
// the session, and CURSOR_INVOKED_AS how the harness was started (the name it
// was run under).
//
// sr:provides subprocess-session-env/cursor
// sr:docs https://cursor.com/docs/hooks#environment-variables
func Identity(sessionID, request, version string) map[string]string {
	return map[string]string{
		"CURSOR_CONVERSATION_ID": sessionID,
		"CURSOR_INVOKED_AS":      "cursor-agent",
		"CURSOR_REQUEST_ID":      request,
		"CURSOR_RIPGREP_PATH":    RipgrepPath(version),
	}
}

// RipgrepPath is where the harness keeps the ripgrep it bundles, for the
// version it is: under the home of the account that installed it, which the
// harness reads from the account (not the HOME a run may have been given), so
// the mock does the same. The mock has no ripgrep to put there (recorded:
// runs/subprocess-session-env, runs/nested-session-env).
func RipgrepPath(version string) string {
	home := ""
	if u, err := user.Current(); err == nil {
		home = u.HomeDir
	}
	return filepath.Join(home, ".local", "share", "cursor-agent", "versions", version, "rg")
}

// Defaults are what a Shell tool command sees only when the harness inherited
// none: CURSOR_AGENT=1 marks "running under the Cursor agent". An inherited
// value passes through (recorded: runs/nested-session-env).
func Defaults() map[string]string {
	return map[string]string{"CURSOR_AGENT": "1"}
}

// HookIdentity is what a hook command sees whatever it inherited: how the
// harness was started, the project dir under Claude Code's name too, and the
// conversation's transcript file once it is named in the payloads (recorded:
// runs/nested-session-env, launched over decoys). A hook runs from the project
// root, which is its working directory by every name: the PWD it sees is the
// root, not the directory (a symlink to it, say) the harness was started from
// (recorded: runs/symlinked-cwd).
//
// sr:provides subprocess-session-env/cursor
// sr:docs https://cursor.com/docs/hooks#environment-variables
func HookIdentity(dir, transcript, version string) map[string]string {
	m := map[string]string{"CURSOR_INVOKED_AS": "cursor-agent", "CLAUDE_PROJECT_DIR": dir, "PWD": dir, "CURSOR_RIPGREP_PATH": RipgrepPath(version)}
	if transcript != "" {
		m["CURSOR_TRANSCRIPT_PATH"] = transcript
	}
	return m
}

// HookDefaults are what a hook command sees only when the harness inherited
// none: the workspace root and the Cursor version. An inherited value passes
// through (recorded: runs/nested-session-env).
func HookDefaults(dir, version string) map[string]string {
	return map[string]string{"CURSOR_PROJECT_DIR": dir, "CURSOR_VERSION": version}
}
