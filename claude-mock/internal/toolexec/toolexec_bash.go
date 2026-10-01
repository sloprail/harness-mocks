package toolexec

import (
	"context"
	"encoding/json"
	"os"
	"strings"

	"github.com/sloprail/harness-mocks/claude-mock/internal/childenv"
	"github.com/sloprail/harness-mocks/internal/procexec"
	"github.com/sloprail/harness-mocks/internal/tools"
)

// bashInput is the argument shape for the Bash tool.
// sr:docs https://docs.anthropic.com/en/docs/claude-code/tools-reference
type bashInput struct {
	Command string `json:"command"`
	Timeout int    `json:"timeout,omitempty"`
}

func executeBash(ctx context.Context, raw json.RawMessage, cwd, sessionID string) Result {
	var inp bashInput
	if err := json.Unmarshal(raw, &inp); err != nil || inp.Command == "" {
		return Result{Output: "Bash: missing or invalid 'command' field", IsError: true}
	}

	ran := tools.Bash(ctx, inp.Command, cwd, bashEnv(sessionID, inp.Command))
	text, failedRun := ran.MessageFor(inp.Command, tools.BenignExit1)
	// toolUseResult/tool_response: the structured result real Claude Code
	// records for a foreground Bash ({stdout, stderr, interrupted, isImage,
	// noOutputExpected} — a claude 2.1.282 PostToolUse payload). The mock runs
	// the command with one combined stream, so stdout carries it all.
	// sr:provides bash-tool-result/claude
	structured := map[string]any{
		"stdout": text, "stderr": "", "interrupted": false, "isImage": false, "noOutputExpected": false,
	}
	if failedRun {
		// A command that exits non-zero is answered the way claude 2.1.28x
		// answers it: "Exit code N" then the output, as an error, with
		// toolUseResult "Error: <that text>" (1,316 real results; a controlled
		// 2.1.282 run; snapshots/runs/bashfail). What the text says is core's
		// (tools.BashResult.Message).
		return failed(text)
	}
	return Result{Output: text, ToolUseResult: structured}
}

// bashEnv is the environment a Bash tool subprocess runs with: the mock's own
// environment plus CLAUDE_CODE_SESSION_ID, which real Claude Code exports into
// every Bash tool subprocess so a command can resolve "the current session"
// (e.g. `sr-session trajectory cite`). The override is appended LAST so it wins
// over any inherited value — without it, a mock run nested inside a live Claude
// Code session would hand its tool calls the OPERATOR's outer session id. Set
// only when non-empty, matching the hook invoker (hooks/invoker.go).
func bashEnv(sessionID, command string) []string {
	ident := childenv.Identity(sessionID)
	// sloprail's own commands resolve their session elsewhere: keep them out of it
	if strings.HasPrefix(strings.TrimSpace(command), "sr-") {
		delete(ident, "CLAUDE_CODE_SESSION_ID")
	}
	return procexec.Env(os.Environ(), ident, childenv.Defaults())
}
