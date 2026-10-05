package runner

import (
	"context"

	"github.com/sloprail/harness-mocks/claude-mock/internal/hooks"
	corehooks "github.com/sloprail/harness-mocks/internal/hooks"
	"github.com/sloprail/harness-mocks/internal/tasks"
)

// Background tasks: a Bash or an Agent called with run_in_background, the
// receipts real Claude Code answers them with, and how a finished one is handed
// back to the agent. Every text and field below is pinned to the claude binary, the
// recorded runs under claude-mock/snapshots/runs (bgbash, bgagent, midturn), and
// the real transcripts on one machine.
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
//
// sr:provides task-notifications/claude
func (b *backgroundTasks) deliverMidTurn(ctx context.Context, cfg Config, inv *hooks.Invoker, tr *transcript) {
	for _, t := range b.TakeFinished(cfg.AgentID) {
		note := taskNotification(t)
		if !submitNotification(ctx, cfg, inv, tr, note) {
			continue
		}
		tr.persistMap(map[string]any{"type": "attachment", "attachment": map[string]any{
			"type": "queued_command", "prompt": note, "source_uuid": newRecordUUID(),
			"commandMode": "task-notification", "timestamp": nowStamp(), "origin": notificationOrigin(),
		}})
		tr.flushHookRuns()
	}
}

// deliverAsTurn writes a finished task's notification as the user turn that
// starts a new turn, once UserPromptSubmit has let it through. It reports
// false — writing nothing, so no turn runs for it — when the hook refused it.
func (b *backgroundTasks) deliverAsTurn(ctx context.Context, cfg Config, inv *hooks.Invoker, tr *transcript, t *tasks.Task) bool {
	note := taskNotification(t)
	if !submitNotification(ctx, cfg, inv, tr, note) {
		return false
	}
	tr.persistMap(map[string]any{
		"type":                 "user",
		"message":              map[string]any{"role": "user", "content": note},
		"origin":               notificationOrigin(),
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
	// sr:provides user-prompt-submit-hook/claude
	if !corehooks.PromptHookFires(corehooks.PromptTaskNotification) {
		return true
	}
	out, err := inv.WithRecorder(tr.holdHookRuns).Fire(ctx, hooks.Input{
		SessionID: cfg.SessionID, Cwd: cfg.Cwd, HookEventName: hooks.EventUserPromptSubmit, Prompt: note, ContinuesPrompt: true,
	})
	if refused, _ := corehooks.PromptOutcome(err != nil || out.Decision == "block", ""); refused {
		tr.dropHeldHookRuns()
		return false
	}
	return true
}

// notificationOrigin is the origin a task notification's record carries, as the
// turn it starts and the attachment it is handed over as both do (recorded:
// snapshots/runs/bgagent and midturn).
func notificationOrigin() map[string]any {
	return map[string]any{"kind": "task-notification", "producer": "session-task"}
}
