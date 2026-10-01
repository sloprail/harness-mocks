package toolexec

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/sloprail/harness-mocks/claude-mock/internal/childenv"
	"github.com/sloprail/harness-mocks/internal/procexec"
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

	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", inp.Command) //nolint:gosec
	cmd.Dir = cwd
	cmd.Env = bashEnv(sessionID)
	out, err := cmd.CombinedOutput()
	text := strings.TrimRight(string(out), "\n")
	// toolUseResult/tool_response: the structured result real Claude Code
	// records for a foreground Bash ({stdout, stderr, interrupted, isImage,
	// noOutputExpected} — a claude 2.1.282 PostToolUse payload). The mock runs
	// the command with one combined stream, so stdout carries it all.
	structured := map[string]any{
		"stdout": text, "stderr": "", "interrupted": false, "isImage": false, "noOutputExpected": false,
	}
	if err != nil {
		// A command that exits non-zero is answered the way claude 2.1.28x
		// answers it: "Exit code N" then the output, as an error, with
		// toolUseResult "Error: <that text>" (1,316 real results; a controlled
		// 2.1.282 run).
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() > 0 {
			msg := fmt.Sprintf("Exit code %d", exitErr.ExitCode())
			if text != "" {
				msg += "\n" + text
			}
			return Result{Output: msg, IsError: true, Failed: true, ToolUseResult: "Error: " + msg}
		}
		return Result{Output: text + "\n" + err.Error(), IsError: true}
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
func bashEnv(sessionID string) []string {
	return procexec.Env(os.Environ(), childenv.Identity(sessionID), childenv.Defaults())
}
