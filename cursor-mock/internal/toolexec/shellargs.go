package toolexec

// ShellFrameArgs are the args a shellToolCall frame carries (recorded: every
// Shell call of runs/*): the command and what Cursor made of it before running it,
// its limits and flags, and the ids of the call, the conversation and the model
// request. A foreground call waits at most its block_until_ms (30000 when it gives
// none) and is moved to the background after it; a call left in the background
// (block_until_ms 0) has no limit and keeps its stdin open. The model's
// description of the call is in the args when it gave one. The bare Args of the
// call (what the transcript records the model called) are not changed.
func (c Call) ShellFrameArgs(callID, conversation, request string) map[string]any {
	p := parseCommand(c.Command())
	writes, devNull := p.outputRedirects()
	parsing := map[string]any{
		"parsingFailed": p.Failed, "executableCommands": p.Commands, "hasRedirects": len(p.Redirects) > 0,
		"hasCommandSubstitution": p.Substitute, "redirects": p.Redirects,
	}
	if len(p.Redirects) > 0 {
		parsing["allRedirectsAreDevNull"] = devNull
	}
	background := c.Background()
	args := map[string]any{
		"command": c.Command(), "workingDirectory": c.str("workingDirectory"), "timeout": 30000,
		"toolCallId": callID, "simpleCommands": p.Names, "hasInputRedirect": p.inputRedirects(), "hasOutputRedirect": writes,
		"parsingResult": parsing, "isBackground": background, "skipApproval": false,
		"adminCommandDenylist": []any{}, "conversationId": conversation, "requestId": request,
	}
	if c.BlockMs != nil {
		args["timeout"] = *c.BlockMs
	}
	if background {
		args["timeout"], args["timeoutBehavior"], args["closeStdin"] = 0, "TIMEOUT_BEHAVIOR_UNSPECIFIED", false
	} else {
		args["timeoutBehavior"], args["closeStdin"] = "TIMEOUT_BEHAVIOR_BACKGROUND", true
		args["hardTimeout"], args["fileOutputThresholdBytes"] = 86400000, "40000"
	}
	if c.Described != "" {
		args["description"] = c.Described
	}
	return args
}
