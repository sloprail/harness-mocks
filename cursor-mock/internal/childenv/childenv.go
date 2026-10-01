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
