package e2e

// notReplaying are the recorded runs whose replay is not green yet, and why.
// The list only shrinks: an entry whose run is gone, or whose run now replays
// green, fails TestGeneratedReplay. "adapter:" is something of the recording
// the cursor adapter cannot reproduce yet; "mock gap:" is what the mock does not
// produce that the recording shows (each gap is a PR of its own). The list as it
// first stood was accepted as it stands, and from then on it may only shrink: no
// entry is added, and an entry may be replaced only when its original gap closes
// and a new one shows.
var notReplaying = map[string]string{
	"compaction-transcript-continuity": "mock gap: beforeReadFile is not fired; afterShellExecution payloads differ in command, output; beforeShellExecution payloads differ in command; postToolUse payloads lack file_path; add command, cwd, timeout; differ in command, tool_name, tool_output; preToolUse payloads lack command, cwd, file_path, timeout; add command, cwd, file_path, timeout; differ in command, file_path, tool_name; readToolCall frames lack contentBlobId, endLine, exceededLimit, fileSize, isEmpty, limit, path, relatedCursorRulePaths, relatedCursorRules, startLine, totalLines; add errorMessage, path; differ in toolCallId; shellToolCall frames lack adminCommandDenylist, closeStdin, conversationId, description, destinationFds, executableCommands.fullText, executableCommands.name, executableCommands.type, executableCommands.value, fileOutputThresholdBytes, hardTimeout, hasInputRedirect, hasOutputRedirect, isBackground, localExecutionTimeMs, operator, outputLocation.filePath, outputLocation.lineCount, outputLocation.sizeBytes, parsingResult.allRedirectsAreDevNull, parsingResult.executableCommands, parsingResult.hasCommandSubstitution, parsingResult.hasRedirects, parsingResult.parsingFailed, parsingResult.redirects, requestId, simpleCommands, skipApproval, targetNodeType, targetText, timeout, timeoutBehavior, toolCallId, workingDirectory; differ in interleavedOutput, stdout, toolCallId; hook log has 126 lines, the mock's 108",
}
