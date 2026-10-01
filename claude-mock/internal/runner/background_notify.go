package runner

import (
	"context"

	"github.com/sloprail/harness-mocks/claude-mock/internal/hooks"
)

// Background tasks: a Bash or an Agent called with run_in_background, the
// receipts real Claude Code answers them with, and how a finished one is handed
// back to the agent. Every text and field below is pinned in EVIDENCE.md to the
// claude 2.1.282 binary, controlled `claude -p` runs of it, and the real
// transcripts on one machine.
//
//   - A background Bash is answered at once with "Command running in
//     background with ID: <id>. Output is being written to: <file>. You will
//     be notified when it completes. To check interim output, use Read on that
//     file path." (plus a cwd note when the command changes directory), with
//     toolUseResult.backgroundTaskId.
//   - A background Agent is answered at once with the "Async agent launched
//     successfully." receipt; its output_file is the sub-agent's own JSONL,
//     reached through <tasks>/<agentId>.output, a symlink. The sub-agent runs
//     concurrently with the turn that launched it.
//   - A task that finishes while its owner is still working is handed over
//     mid-turn: a queued_command attachment (commandMode "task-notification")
//     after the next tool result.
//   - At the end of a turn Stop fires with the still-running tasks in
//     background_tasks. A `claude -p` session then waits for its background
//     AGENTS: each finished task becomes a new user turn (origin
//     task-notification), the agent answers it, and Stop fires again. A
//     background Bash still running when nothing else is left is killed; its
//     stopped notification goes only to the output stream, never the
//     transcript.
//
// Tasks live in one registry per session (sub-agents share their parent's):
// real Stop/SubagentStop payloads list the whole session's tasks, while each
// notification goes to the agent that launched the task.

// agentNotificationNote is the <note> a background Agent's notification
// carries when it stops with no background work of its own still running.
const agentNotificationNote = "A task-notification fires each time this agent stops with no live background children of its own. The user can send it another message and resume it, so the same task-id may notify more than once."

// writeTaskEndFrames writes what a real `claude -p --output-format
// stream-json` run streams when a background command ends: task_updated
// {patch:{status, end_time}}, then task_notification {status, output_file,
// summary}. A command killed at the end of a session is "killed", then
// "stopped" with its description as summary (F:bgbash); one that finished is
// "completed"/"failed" with the notification's summary (F:midturn).
func writeTaskEndFrames(cfg Config, t *backgroundTask) {
	updated := t.status()
	summary := t.summary()
	if t.killed.Load() {
		updated, summary = "killed", t.description
	}
	writeTaskUpdated(cfg, t.id, updated)
	writeTaskNotification(cfg, taskNote{ID: t.id, ToolUseID: t.toolUseID, Status: t.status(), OutputFile: t.outputFile, Summary: summary})
}

// writeFrame writes a system frame to the session's output stream, stamped
// with a uuid and the session id as real frames are.
func writeFrame(cfg Config, frame map[string]any) {
	frame["uuid"] = newRecordUUID()
	frame["session_id"] = cfg.SessionID
	line, err := marshalRecord(frame)
	if err != nil {
		return
	}
	w := cfg.stream
	if w == nil {
		w = cfg.Out
	}
	w.Write(append(line, '\n')) //nolint:errcheck // one Write per line; see writeStreamLine
}

// deliverMidTurn hands owner's finished tasks over inside the running turn:
// a queued_command attachment per task (commandMode "task-notification"),
// written after the tool result it arrived during, and UserPromptSubmit fired
// with the notification as its prompt — as claude 2.1.282 did in a controlled
// `claude -p` run. A notification the hook refuses is not handed over.
func (b *backgroundTasks) deliverMidTurn(ctx context.Context, cfg Config, inv *hooks.Invoker, tr *transcript) {
	for _, t := range b.takeFinished(cfg.AgentID) {
		note := t.notification()
		if !submitNotification(ctx, cfg, inv, tr, note) {
			continue
		}
		tr.persistMap(map[string]any{"type": "attachment", "attachment": map[string]any{
			"type": "queued_command", "prompt": note, "source_uuid": newRecordUUID(),
			"commandMode": "task-notification", "timestamp": nowStamp(),
		}})
		tr.flushHookRuns()
	}
}

// deliverAsTurn writes a finished task's notification as the user turn that
// starts a new turn, once UserPromptSubmit has let it through. It reports
// false — writing nothing, so no turn runs for it — when the hook refused it.
func (b *backgroundTasks) deliverAsTurn(ctx context.Context, cfg Config, inv *hooks.Invoker, tr *transcript, t *backgroundTask) bool {
	note := t.notification()
	if !submitNotification(ctx, cfg, inv, tr, note) {
		return false
	}
	tr.persistMap(map[string]any{
		"type":                 "user",
		"message":              map[string]any{"role": "user", "content": note},
		"origin":               map[string]any{"kind": "task-notification"},
		"promptSource":         "system",
		"turnOrigin":           "task_notification",
		"queueSkipAttachments": true,
	})
	tr.flushHookRuns()
	return true
}

// submitNotification fires UserPromptSubmit for a notification, holding what
// its hooks leave until the notification itself is written (they follow it),
// and reports whether they let it through. A refused notification leaves
// nothing.
func submitNotification(ctx context.Context, cfg Config, inv *hooks.Invoker, tr *transcript, note string) bool {
	out, err := inv.WithRecorder(tr.holdHookRuns).Fire(ctx, hooks.Input{
		SessionID: cfg.SessionID, Cwd: cfg.Cwd, HookEventName: hooks.EventUserPromptSubmit, Prompt: note,
	})
	if err != nil || out.Decision == "block" {
		tr.dropHeldHookRuns()
		return false
	}
	return true
}
