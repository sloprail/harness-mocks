package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sloprail/harness-mocks/claude-mock/internal/runner"
)

// lookedUp is whether the run's --resume value was found by lookup.
func lookedUp(cmd *cobra.Command) bool {
	b, _ := cmd.Flags().GetBool(flagResumeLookup)
	return b
}

// permissionMode is the mode the run's hooks are told it is in:
// bypassPermissions for --dangerously-skip-permissions, else the
// --permission-mode given, else default.
// sr:docs https://code.claude.com/docs/en/hooks#common-input-fields
func permissionMode(cmd *cobra.Command) string {
	if skip, _ := cmd.Flags().GetBool("dangerously-skip-permissions"); skip {
		return "bypassPermissions"
	}
	if m, _ := cmd.Flags().GetString("permission-mode"); m != "" {
		return m
	}
	return "default"
}

// refuseUnimplemented is the error for an input the mock does not implement
// (adr/fail-fast-unimplemented): an output format but stream-json, which a run
// would otherwise print stream frames for as if it were one;
// --include-partial-messages; and a piped stdin beside a prompt argument (real
// claude combines the two; a piped stdin alone is the prompt, see stdinPrompt).
func refuseUnimplemented(cmd *cobra.Command, args []string) error {
	if f, _ := cmd.Flags().GetString(flagOutputFormat); f != "stream-json" {
		return fmt.Errorf("claude-mock: --output-format %s is not implemented by the mock (only stream-json): it is refused rather than ignored", f)
	}
	for _, name := range []string{"include-partial-messages", "input-format", "max-budget-usd", "bare", "agent"} {
		if cmd.Flags().Changed(name) {
			return fmt.Errorf("claude-mock: --%s is not implemented by the mock: it is refused rather than ignored", name)
		}
	}
	if v, _ := cmd.Flags().GetString("permission-prompt-tool"); v != "" && v != "stdio" {
		return fmt.Errorf("claude-mock: --permission-prompt-tool %s is not implemented by the mock (only stdio): it is refused rather than ignored", v)
	}
	// --name names a session this run starts; naming a resumed one (a rename) is not recorded
	if cmd.Flags().Changed("name") && (cmd.Flags().Changed(flagResume) || cmd.Flags().Changed(flagContinue)) {
		return fmt.Errorf("claude-mock: --name with --resume or --continue is not implemented by the mock: it is refused rather than ignored")
	}
	if len(args) > 0 && stdinGiven() {
		return fmt.Errorf("claude-mock: a piped stdin together with a prompt argument is not implemented by the mock: it is refused rather than ignored")
	}
	return nil
}

// stdinGiven is whether stdin is a pipe or a file with content rather than a terminal or /dev/null.
func stdinGiven() bool {
	fi, err := os.Stdin.Stat()
	return err == nil && (fi.Mode()&os.ModeNamedPipe != 0 || fi.Mode().IsRegular() && fi.Size() > 0)
}

// stdinPrompt is the prompt `claude -p` reads from a piped stdin when it is given no prompt
// argument, minus the trailing newline of the pipe.
// sr:docs https://code.claude.com/docs/en/headless#basic-usage
func stdinPrompt() (string, error) {
	if !stdinGiven() {
		return "", nil
	}
	b, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", fmt.Errorf("claude-mock: read stdin: %w", err)
	}
	return strings.TrimRight(string(b), "\r\n"), nil
}

// addRefusedFlags registers the flags of claude the mock refuses by name: --agent (it would put
// agent_type on main-thread hook payloads) and --bare (it skips hooks, settings discovery and
// the login; the run recorded without an API key only fails to log in, snapshots/runs/bare, so
// what a bare run does is not recorded).
// sr:docs https://code.claude.com/docs/en/headless#start-faster-with-bare-mode
func addRefusedFlags(cmd *cobra.Command) {
	cmd.Flags().Bool("bare", false, "Refused: not implemented by the mock")
	cmd.Flags().String("agent", "", "Refused: not implemented by the mock")
}

// flagError words a flag the mock does not know as claude does: `error: unknown option '--x'`
// on stderr, nothing on stdout, exit status 1, before the run starts (recorded: snapshots/runs/invalid-flag).
// Only a long flag's wording is recorded: a short one (-x) keeps the mock's own.
// sr:docs https://code.claude.com/docs/en/headless#basic-usage
func flagError(_ *cobra.Command, err error) error {
	if name, ok := strings.CutPrefix(err.Error(), "unknown flag: "); ok {
		return fmt.Errorf("error: unknown option '%s'", name)
	}
	return err
}

// maxTurns is --max-turns (0: none).
func maxTurns(cmd *cobra.Command) int {
	n, _ := cmd.Flags().GetInt("max-turns")
	return n
}

// hookEvents is --include-hook-events.
func hookEvents(cmd *cobra.Command) bool {
	b, _ := cmd.Flags().GetBool("include-hook-events")
	return b
}

// invocation is the run's --name and --tools: the names the latter lists, and whether it restricts the
// run's tools at all ("default" does not).
func invocation(cmd *cobra.Command) runner.Invocation {
	name, _ := cmd.Flags().GetString("name")
	value, _ := cmd.Flags().GetString("tools")
	host, _ := cmd.Flags().GetString("permission-prompt-tool")
	if !cmd.Flags().Changed("tools") || value == "default" {
		return runner.Invocation{Name: name, PermissionHost: host == "stdio"}
	}
	return runner.Invocation{Name: name, PermissionHost: host == "stdio", RestrictTools: true, Tools: strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == ' ' })}
}

// noConversationResult is the result frame of a run that resumed a session with no transcript: no turn
// was taken and nothing was denied (recorded: snapshots/runs/resume-unknown).
func noConversationResult(sessionID, msg string) map[string]any {
	return map[string]any{
		"type": "result", "subtype": "error_during_execution", "is_error": true,
		"num_turns": 0, "session_id": sessionID, "errors": []string{msg},
		"permission_denials": []any{}, "result_index": 0, "stop_reason": nil,
	}
}

// addInvocationFlags registers --tools (the tools the run has: a call to another is refused;
// cli-reference#--tools, recorded in snapshots/runs/file-tools) and --name (the session carries a name,
// and --resume <name> finds it; cli-reference#--name, recorded in snapshots/runs/resume-name).
func addInvocationFlags(cmd *cobra.Command) {
	cmd.Flags().String("tools", "", `The only tools the run has: "default" for all, "" for none, else names separated by commas or spaces`)
	cmd.Flags().StringP("name", "n", "", "Name the session (--name, -n, as used by claude CLI)")
	// --permission-prompt-tool stdio: the run has a permission host, which is what offers AskUserQuestion to a
	// non-interactive run (hooks#defer-a-tool-call-for-later, recorded in snapshots/runs/ask-user-question-tool).
	// Another tool (an MCP one) is not implemented.
	cmd.Flags().String("permission-prompt-tool", "", `The permission host of the run: "stdio" (an MCP tool is not implemented)`)
}
