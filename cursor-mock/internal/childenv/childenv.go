// Package childenv is Cursor's names for the harness and the session in the
// environment of a process it starts (a Shell tool command).
package childenv

// Identity is what a Shell tool command sees whatever it inherited (recorded:
// runs/nested-session-env, launched over decoys): CURSOR_CONVERSATION_ID names
// the session, and CURSOR_INVOKED_AS how the harness was started (the name it
// was run under).
//
// sr:provides subprocess-session-env/cursor
// sr:docs https://cursor.com/docs/hooks#environment-variables
func Identity(sessionID string) map[string]string {
	return map[string]string{
		"CURSOR_CONVERSATION_ID": sessionID,
		"CURSOR_INVOKED_AS":      "cursor-agent",
	}
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
// runs/nested-session-env, launched over decoys).
//
// sr:provides subprocess-session-env/cursor
// sr:docs https://cursor.com/docs/hooks#environment-variables
func HookIdentity(dir, transcript string) map[string]string {
	m := map[string]string{"CURSOR_INVOKED_AS": "cursor-agent", "CLAUDE_PROJECT_DIR": dir}
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
