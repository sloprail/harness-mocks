// Package toolexec executes Claude Code tool calls in-process during mock runs.
//
// The mock uses a turn-based protocol: when the scenario script emits an
// assistant record containing a tool_use block, the mock executes the tool
// locally and then re-invokes the script so it can emit the next turn based
// on the tool's result.
//
// Only the subset of tools needed for realistic e2e scenario scripts is
// implemented here. Unknown tools return an error result so the script can
// handle them explicitly.
//
// sr:docs https://docs.anthropic.com/en/docs/claude-code/tools-overview
package toolexec

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
)

// Result is the output of a tool execution.
type Result struct {
	// Output is the text content returned to the model.
	Output string
	// IsError is true when the tool execution failed and the output is an error message.
	IsError bool
	// ToolUseResult is the structured result real Claude Code records beside a
	// tool_result in the transcript (the record's toolUseResult field), where the
	// tool has one — a background launch's backgroundTaskId, an async agent's
	// agentId. Nil for the tools that do not need it here.
	ToolUseResult any
	// Failed marks a tool that ran and failed (a Bash that exited non-zero, a
	// file tool's error), for which real Claude Code fires PostToolUseFailure;
	// input the tool could not take is an error that did not run.
	Failed bool
	// ContentAsBlocks writes the tool_result content as a list of text blocks
	// rather than a string — the shape real Claude Code gives some tools'
	// results (an async Agent receipt).
	ContentAsBlocks bool
}

// Execute runs the named tool with the given JSON input and returns its result.
// cwd is the working directory for tools that operate on the filesystem.
// sessionID is the active session id; the Bash tool exports it to its subprocess
// as CLAUDE_CODE_SESSION_ID, as real Claude Code does (see executeBash).
//
// sr:docs https://docs.anthropic.com/en/docs/claude-code/tools-overview
func Execute(ctx context.Context, toolName string, input json.RawMessage, cwd, sessionID string) Result {
	switch toolName {
	case "Bash":
		return executeBash(ctx, input, cwd, sessionID)
	case "Read":
		return executeRead(input, cwd)
	case "Write":
		return executeWrite(input, cwd)
	case "Edit":
		return executeEdit(input, cwd)
	case "Glob":
		return executeGlob(input, cwd)
	case "ToolSearch":
		// The mock has no deferred tools, so none matches (recorded: runs/fgsub-tool-stats).
		return Result{Output: "No matching deferred tools found"}
	default:
		return Result{
			Output:  fmt.Sprintf("tool %q is not implemented in the mock", toolName),
			IsError: true,
		}
	}
}

func resolvePath(p, cwd string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(cwd, p)
}

// failed is the result of a tool that ran and failed: the error is the text
// the agent gets, and the transcript records it as "Error: <text>" (claude
// 2.1.285, recorded: snapshots/runs/bashfail, tool-errors).
func failed(msg string) Result {
	return Result{Output: msg, IsError: true, Failed: true, ToolUseResult: "Error: " + msg}
}
