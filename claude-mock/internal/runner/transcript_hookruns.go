package runner

import (
	"github.com/sloprail/harness-mocks/claude-mock/internal/hooks"
	corehooks "github.com/sloprail/harness-mocks/internal/hooks"
)

// recordHookRuns writes what real Claude Code records for one fired hook
// event, handler by handler. Each rule below is pinned to evidence: real
// transcripts, the recorded runs under claude-mock/snapshots/runs (hookmix,
// hook-exit-codes, hookerrors), and the claude binary's hook runner:
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
//   - a hook its timeout cancelled: hook_cancelled {command, durationMs,
//     timedOut, timeoutMs}; an HTTP hook that failed: hook_non_blocking_error
//     {stderr: the failure, stdout "", exitCode 0} (snapshots/runs/hook-timeout,
//     http-hook). A prompt hook's JSON context leaves only its
//     hook_additional_context (snapshots/runs/ctxmulti).
//
// What each run leaves is core's (corehooks.RecordFor); this writes it in
// Claude Code's record shapes.
//
// Every attachment carries hookName (see hookRunName), hookEvent and toolUseID
// — the tool call's id for a tool event, else one fresh uuid for the whole fire. A Stop fire that ran any handler ends with a stop_hook_summary
// record carrying the same toolUseID.
func (t *transcript) recordHookRuns(in hooks.Input, runs []hooks.HandlerRun) {
	ev := in.HookEventName
	if t == nil || !corehooks.LeavesRecords(momentOf(ev)) {
		return
	}
	hookName := hookRunName(in)
	toolUseID := in.ToolUseID
	if toolUseID == "" {
		toolUseID = newRecordUUID()
	}
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
		// sr:provides hook-output-transcript-records/claude
		rec := corehooks.RecordFor(ranOf(ev, r, ac))
		switch {
		case rec.Attachment == corehooks.AttachCancelled:
			att("hook_cancelled", map[string]any{
				"command": r.Command, "durationMs": r.DurationMs, "timedOut": true, "timeoutMs": r.TimeoutMs,
			})
			info["durationMs"] = r.DurationMs
		case r.Blocked:
			t.recordExit2(att, &summary, ev, r, rec)
		case rec.Attachment == corehooks.AttachBlockingError: // exit 0, blocking by its JSON
			reason := r.Output.Reason
			if reason == "" {
				reason = "Blocked by hook"
			}
			if rec.Feedback {
				t.stopHookFeedback(reason)
			}
			att("hook_blocking_error", map[string]any{
				"blockingError": map[string]any{"blockingError": reason, "command": r.Command},
			})
			summary.errors = append(summary.errors, reason)
			summary.hasOutput = true
		case rec.Attachment == corehooks.AttachNonBlockingError:
			msg := nonBlockingMessage(r)
			fields := map[string]any{"stderr": msg, "stdout": r.Stdout, "exitCode": r.ExitCode, "command": r.Command, "durationMs": r.DurationMs}
			if r.HTTPError != "" {
				fields = map[string]any{"stderr": msg, "stdout": "", "exitCode": 0}
			}
			att("hook_non_blocking_error", fields)
			info["durationMs"] = r.DurationMs
			summary.errors = append(summary.errors, msg)
			summary.hasOutput = true
		case rec.Attachment == corehooks.AttachSuccess || rec.Context:
			if t.recordSuccess(att, in, r, hookName, toolUseID, ac, rec) {
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
	if corehooks.SummaryAfter(momentOf(ev), len(runs)) {
		t.writeStopSummary(summary)
	}
}
