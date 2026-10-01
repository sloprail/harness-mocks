package runner

import (
	"fmt"
	"strings"

	"github.com/sloprail/harness-mocks/claude-mock/internal/hooks"
)

// recordHookRuns writes what real Claude Code records for one fired hook
// event, handler by handler. Each rule below is pinned to evidence in
// claude-mock/EVIDENCE.md (real transcripts, controlled runs of claude
// 2.1.282, and the 2.1.282 binary's hook runner):
//
//   - exit 0, nothing printed: no record at all.
//   - exit 0 with output: hook_success {content, stdout, stderr, exitCode,
//     command, durationMs}; content is stdout when it is plain text, "" when
//     stdout is a JSON object. A JSON additionalContext then adds a
//     hook_additional_context {content: [text]} right after it.
//   - exit 0 with a JSON block — Stop/SubagentStop decision:block, PostToolUse
//     decision:block: the "Stop hook feedback" meta turn (Stop events) and a
//     hook_blocking_error {blockingError: {blockingError: reason, command}}.
//     A PreToolUse deny records nothing: the refusal is the tool_result.
//   - exit 2: SessionStart and SubagentStart write it as a
//     hook_non_blocking_error whose stderr is "[<command>]: <stderr>" (no
//     durationMs); Stop/SubagentStop write only the feedback meta turn
//     "Stop hook feedback:\n[<command>]: <stderr>"; PostToolUse and
//     PostToolUseFailure write a hook_blocking_error (the latter recorded in
//     snapshots/runs/hook-exit-codes); PreToolUse writes nothing (its tool_result carries
//     it). Other events: no evidence, nothing written.
//   - any other non-zero exit: hook_non_blocking_error {stderr: "Failed with
//     non-blocking status code: <stderr or No stderr output>", stdout,
//     exitCode, command, durationMs}.
//
// Every attachment carries hookName (see hookRunName), hookEvent and toolUseID
// — the tool call's id for a tool event, else one fresh uuid for the whole fire. A Stop fire that ran any handler ends with a stop_hook_summary
// record carrying the same toolUseID.
func (t *transcript) recordHookRuns(in hooks.Input, runs []hooks.HandlerRun) {
	if t == nil || !recordedEvents[in.HookEventName] {
		return
	}
	hookName := hookRunName(in)
	toolUseID := in.ToolUseID
	if toolUseID == "" {
		toolUseID = newRecordUUID()
	}
	ev := in.HookEventName
	stopLike := ev == hooks.EventStop || ev == hooks.EventSubagentStop
	summary := stopSummary{toolUseID: toolUseID}
	att := func(typ string, fields map[string]any) {
		a := map[string]any{"type": typ, "hookName": hookName, "toolUseID": toolUseID, "hookEvent": string(ev)}
		for k, v := range fields {
			a[k] = v
		}
		t.persistMap(map[string]any{"type": "attachment", "attachment": a})
	}
	for _, r := range runs {
		info := map[string]any{"command": r.Command}
		ac := additionalContextFrom(r.Output)
		switch {
		case r.Blocked:
			quoted := hooks.QuoteRun(r)
			switch ev {
			case hooks.EventSessionStart, hooks.EventSubagentStart:
				att("hook_non_blocking_error", map[string]any{
					"stderr": quoted, "stdout": r.Stdout, "exitCode": r.ExitCode, "command": r.Command,
				})
			case hooks.EventStop, hooks.EventSubagentStop:
				t.stopHookFeedback(quoted)
				summary.errors = append(summary.errors, quoted)
			case hooks.EventPostToolUse, hooks.EventPostToolUseFailure: // the tool already ran; stderr is shown to Claude (docs)
				att("hook_blocking_error", map[string]any{
					"blockingError": map[string]any{"blockingError": quoted, "command": r.Command},
				})
			}
			summary.hasOutput = true
		case (stopLike || ev == hooks.EventPostToolUse) && r.Output.Decision == "block":
			reason := r.Output.Reason
			if reason == "" {
				reason = "Blocked by hook"
			}
			if stopLike {
				t.stopHookFeedback(reason)
			}
			att("hook_blocking_error", map[string]any{
				"blockingError": map[string]any{"blockingError": reason, "command": r.Command},
			})
			summary.errors = append(summary.errors, reason)
			summary.hasOutput = true
		case ev == hooks.EventPreToolUse && isDeny(r.Output):
			// The refusal is the tool_result (see scanLines); nothing else.
			info["durationMs"] = r.DurationMs
		case r.JSONError != "":
			// recorded in snapshots/runs/hook-exit-json: a non-blocking error
			// whose stderr is the parse or validation message (and, on a
			// non-zero exit, the hook's own stderr after it)
			msg := r.JSONError
			if r.ExitCode != 0 {
				msg += fmt.Sprintf("\n\nHook exited %d with stderr:\n%s", r.ExitCode, strings.TrimSpace(r.Stderr))
			}
			att("hook_non_blocking_error", map[string]any{
				"stderr": msg, "stdout": r.Stdout, "exitCode": r.ExitCode, "command": r.Command, "durationMs": r.DurationMs,
			})
			info["durationMs"] = r.DurationMs
			summary.errors = append(summary.errors, msg)
			summary.hasOutput = true
		case r.ExitCode != 0 && !r.JSONParsed:
			msg := strings.TrimSpace(r.Stderr)
			if msg == "" {
				msg = "No stderr output"
			}
			msg = "Failed with non-blocking status code: " + msg
			att("hook_non_blocking_error", map[string]any{
				"stderr": msg, "stdout": r.Stdout, "exitCode": r.ExitCode, "command": r.Command, "durationMs": r.DurationMs,
			})
			info["durationMs"] = r.DurationMs
			summary.errors = append(summary.errors, msg)
			summary.hasOutput = true
		case r.Stdout != "" || r.Stderr != "":
			content := ""
			if !r.JSONParsed { // plain text, as the adapter read it
				content = strings.TrimRight(r.Stdout, "\n")
			}
			att("hook_success", map[string]any{
				"content": content, "stdout": r.Stdout, "stderr": r.Stderr, "exitCode": r.ExitCode,
				"command": r.Command, "durationMs": r.DurationMs,
			})
			if ac != "" {
				t.additionalContext(in, hookName, toolUseID, ac)
				summary.contexts = append(summary.contexts, ac)
			}
			info["durationMs"] = r.DurationMs
			summary.hasOutput = true
		default:
			info["durationMs"] = r.DurationMs
		}
		if r.Output.Continue != nil && !*r.Output.Continue {
			summary.prevented = true
			summary.stopReason = r.Output.StopReason
			if summary.stopReason == "" {
				summary.stopReason = "Stop hook prevented continuation"
			}
		}
		summary.infos = append(summary.infos, info)
	}
	if ev == hooks.EventStop {
		t.writeStopSummary(summary)
	}
}
