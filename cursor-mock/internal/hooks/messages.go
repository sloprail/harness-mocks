package hooks

// workaroundNote closes what the agent is told about a refused call.
const workaroundNote = "Agent note: Do not suggest workarounds to the blocked tool."

// PreToolRefusal is how a call refused at preToolUse is reported, from the
// hooks' message: the failure hook's error_message is the message itself, and
// the tool's result to the agent adds the note not to look for workarounds.
//
// sr:docs https://cursor.com/docs/hooks#pretooluse
func PreToolRefusal(message string) (failure, result string) {
	return message, message + "\n\n" + workaroundNote
}

// ShellRefusal is how a shell command refused at beforeShellExecution is
// reported: one text, for the failure hook's error_message and the tool's
// result alike.
//
// sr:docs https://cursor.com/docs/hooks#beforeshellexecution--beforemcpexecution
func ShellRefusal(message string) (failure, result string) {
	text := "Command execution was blocked by a hook: " + message +
		"\n\nTo view or modify configured hooks, go to Cursor Settings > Hooks.\n\n" + workaroundNote
	return text, text
}

// ReadRefusal is how a file read refused at beforeReadFile is reported: one
// text, for the failure hook's error_message and the tool's result alike
// (recorded: runs/before-read-refusal).
//
// sr:docs https://cursor.com/docs/hooks#beforereadfile
func ReadRefusal(message string) (failure, result string) {
	text := "File read was blocked by a hook: " + message +
		"\n\nTo view or modify configured hooks, go to Cursor Settings > Hooks.\n\n" + workaroundNote
	return text, text
}
