package runner

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/sloprail/harness-mocks/claude-mock/internal/hooks"
	"github.com/sloprail/harness-mocks/claude-mock/internal/toolexec"
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

type backgroundTask struct {
	id          string
	toolUseID   string
	owner       string // agent id of the launcher; "" for the main thread
	agent       bool
	description string
	command     string // a Bash task's command
	agentType   string // an Agent task's type
	outputFile  string
	started     time.Time

	done     chan struct{}
	exitCode int
	killed   atomic.Bool
	// an Agent task's outcome
	result     string
	failure    string
	toolUses   int
	durationMs int64

	cmd *exec.Cmd

	delivered bool // guarded by backgroundTasks.mu
}

func (t *backgroundTask) finished() bool {
	select {
	case <-t.done:
		return true
	default:
		return false
	}
}

type backgroundTasks struct {
	mu      sync.Mutex
	tasks   []*backgroundTask
	changed chan struct{} // signalled (non-blocking) whenever a task finishes
	wg      sync.WaitGroup
	cancel  context.CancelFunc
	ctx     context.Context
}

func newBackgroundTasks() *backgroundTasks {
	ctx, cancel := context.WithCancel(context.Background())
	return &backgroundTasks{changed: make(chan struct{}, 1), ctx: ctx, cancel: cancel}
}

func (b *backgroundTasks) add(t *backgroundTask) {
	b.mu.Lock()
	b.tasks = append(b.tasks, t)
	b.mu.Unlock()
}

func (b *backgroundTasks) finish(t *backgroundTask) {
	close(t.done)
	select {
	case b.changed <- struct{}{}:
	default:
	}
}

// runsInBackground reports whether a tool input asks to run in the background.
// Accepted as a JSON boolean or the string "true" (scenario builders that
// encode every input value as a string).
func runsInBackground(input json.RawMessage) bool {
	var in struct {
		RunInBackground any `json:"run_in_background"`
	}
	if json.Unmarshal(input, &in) != nil {
		return false
	}
	switch v := in.RunInBackground.(type) {
	case bool:
		return v
	case string:
		return v == "true"
	}
	return false
}

// tasksDir is where a session's background task output lives, as real Claude
// Code lays it out: <tmp>/claude-<uid>/<encoded cwd>/<session>/tasks, with
// <tmp> CLAUDE_CODE_TMPDIR or /tmp (the 2.1.282 binary's temp-root function),
// symlinks resolved (real receipts name /private/tmp/… on macOS).
func tasksDir(cwd, sessionID string) string {
	base := os.Getenv("CLAUDE_CODE_TMPDIR")
	if base == "" {
		base = "/tmp"
	}
	root := filepath.Join(base, "claude-"+strconv.Itoa(os.Getuid()))
	_ = os.MkdirAll(root, 0o700)
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	encoded := nonAlphanumRe.ReplaceAllString(resolveEncodingCwd(cwd), "-")
	return filepath.Join(root, encoded, sessionID, "tasks")
}

// changesDirectory reports whether a command has a subcommand that changes
// directory (cd, pushd, popd, chdir) — when real Claude Code adds the
// "Session cwd remains …" note to a background receipt.
func changesDirectory(command string) bool {
	for _, part := range strings.FieldsFunc(command, func(r rune) bool {
		return r == ';' || r == '&' || r == '|' || r == '\n'
	}) {
		fields := strings.Fields(part)
		if len(fields) == 0 {
			continue
		}
		switch fields[0] {
		case "cd", "pushd", "popd", "chdir":
			return true
		}
	}
	return false
}

// launchBash starts a background Bash for owner and returns its receipt.
// endsWithFinalResponse is true inside a foreground sub-agent, whose
// background commands real Claude Code kills when it gives its final response
// (and says so in the receipt).
func (b *backgroundTasks) launchBash(cfg Config, toolUseID string, raw json.RawMessage) toolexec.Result {
	var in struct {
		Command     string `json:"command"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(raw, &in); err != nil || in.Command == "" {
		return inputValidationError("Bash", []string{"command"})
	}
	id := "b" + randomID(8)
	dir := tasksDir(cfg.Cwd, cfg.SessionID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return toolexec.Result{Output: fmt.Sprintf("Bash: %v", err), IsError: true}
	}
	outFile := filepath.Join(dir, id+".output")
	out, err := os.Create(outFile)
	if err != nil {
		return toolexec.Result{Output: fmt.Sprintf("Bash: %v", err), IsError: true}
	}
	desc := in.Description
	if desc == "" {
		desc = in.Command
	}
	task := &backgroundTask{
		id: id, toolUseID: toolUseID, owner: cfg.AgentID, description: desc, command: in.Command,
		outputFile: outFile, started: time.Now(), done: make(chan struct{}),
	}

	// Not bound to the turn's context: a background command outlives the turn
	// that started it. Its own process group, so ending it reaches whatever it
	// spawned.
	cmd := exec.Command("/bin/sh", "-c", in.Command) //nolint:gosec
	cmd.Dir = cfg.Cwd
	cmd.Env = append(os.Environ(), "CLAUDE_CODE_SESSION_ID="+cfg.SessionID)
	cmd.Stdout = out
	cmd.Stderr = out
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		out.Close()
		return toolexec.Result{Output: fmt.Sprintf("Bash: %v", err), IsError: true}
	}
	task.cmd = cmd
	b.add(task)
	writeFrame(cfg, map[string]any{
		"type": "system", "subtype": "task_started", "task_id": id, "tool_use_id": toolUseID,
		"description": desc, "is_backgrounded": true, "task_type": "local_bash",
	})
	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
		err := cmd.Wait()
		code := 0
		if err != nil {
			code = 1
			if cmd.ProcessState != nil && cmd.ProcessState.ExitCode() >= 0 {
				code = cmd.ProcessState.ExitCode()
			}
		}
		task.exitCode = code
		// Real Claude Code appends how the command ended to its output file.
		if task.killed.Load() {
			fmt.Fprint(out, "\n[killed]\n")
		} else {
			fmt.Fprintf(out, "\n[exited with code %d]\n", code)
		}
		out.Close()
		writeTaskEndFrames(cfg, task)
		b.finish(task)
	}()

	endsWithFinal := cfg.SyncSubagent
	parts := []string{"Command running in background with ID: " + id + ". Output is being written to: " + outFile + "."}
	if endsWithFinal {
		parts = append(parts, "If it exits while you are still working you will be notified, but it is terminated when you give your final response and no notification can follow that — so do not end your turn to wait for it; if you need its result, wait for it before giving your final response.")
	} else {
		parts = append(parts, "You will be notified when it completes.")
	}
	parts = append(parts, "To check interim output, use Read on that file path.")
	text := strings.Join(parts, " ")
	tur := map[string]any{
		"stdout": "", "stderr": "", "interrupted": false, "isImage": false, "noOutputExpected": false,
		"backgroundTaskId": id,
	}
	if endsWithFinal {
		tur["backgroundEndsWithFinalResponse"] = true
	}
	if changesDirectory(in.Command) {
		hint := "Session cwd remains " + cfg.Cwd + "; directory changes made by the backgrounded command do not apply to subsequent commands."
		text += "\n" + hint
		tur["backgroundCwdHint"] = hint
	}
	return toolexec.Result{Output: text, ToolUseResult: tur}
}

// launchAgent validates a background Agent call, prepares its sub-agent (its
// sidechain file, and the output-file symlink to it), and returns the receipt
// together with start, which runs the sub-agent concurrently. The caller
// starts it once the receipt's tool_result and PostToolUse are written — real
// Claude Code returns the receipt first and fires SubagentStart after.
func (b *backgroundTasks) launchAgent(cfg Config, inv *hooks.Invoker, toolUseID string, raw json.RawMessage, tr *transcript) (toolexec.Result, func()) {
	sub, in, errRes := prepareSubagent(cfg, toolUseID, raw, tr, true)
	if sub == nil {
		return errRes, nil
	}
	outFile := sub.outputFile

	task := &backgroundTask{
		id: sub.agentID, toolUseID: toolUseID, owner: cfg.AgentID, agent: true,
		description: in.Description, agentType: sub.agentType, outputFile: outFile,
		started: time.Now(), done: make(chan struct{}),
	}
	b.add(task)

	model := in.Model
	if model == "" {
		model = cfg.Model
	}
	if model == "" {
		model = "default"
	}
	text := "Async agent launched successfully. (This tool result is internal metadata — never quote or paste any part of it, including the agentId below, into a user-facing reply.)\n" +
		"agentId: " + sub.agentID + " (internal ID - do not mention to user. Use SendMessage with to: '" + sub.agentID + "', summary: '<5-10 word recap>' to continue this agent.)\n" +
		"The agent is working in the background. You will be notified automatically when it completes. You know nothing about its results until that notification arrives — do not report, assume, or predict them; continue other work or respond to the user in the meantime.\n" +
		"Do not duplicate this agent's work — avoid working with the same files or topics it is using.\n" +
		"output_file: " + outFile + "\n" +
		"Do NOT Read or tail this file via the shell tool — it is the full subagent JSONL transcript and reading it will overflow your context. If the user asks for progress, say the agent is still running; you'll get a completion notification."
	res := toolexec.Result{
		Output:          text,
		ContentAsBlocks: true,
		ToolUseResult: map[string]any{
			"isAsync": true, "status": "async_launched", "agentId": sub.agentID,
			"description": in.Description, "prompt": in.Prompt, "outputFile": outFile,
			"canReadOutputFile": true, "resolvedModel": model,
		},
	}
	start := func() {
		b.wg.Add(1)
		go func() {
			defer b.wg.Done()
			out := sub.execute(b.ctx, inv, b, in.Prompt)
			task.result, task.failure = out.finalText, out.failure
			task.toolUses, task.durationMs = out.toolUses, time.Since(task.started).Milliseconds()
			if out.failure != "" {
				task.exitCode = 1
			}
			b.finish(task)
		}()
	}
	return res, start
}

// running is the session's still-running tasks, as a Stop/SubagentStop
// payload's background_tasks lists them.
func (b *backgroundTasks) running() []hooks.BackgroundTask {
	out := []hooks.BackgroundTask{}
	if b == nil {
		return out
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, t := range b.tasks {
		if t.finished() {
			continue
		}
		if t.agent {
			out = append(out, hooks.BackgroundTask{ID: t.id, Type: "subagent", Status: "running", Description: t.description, AgentType: t.agentType})
		} else {
			out = append(out, hooks.BackgroundTask{ID: t.id, Type: "shell", Status: "running", Description: t.description, Command: t.command})
		}
	}
	return out
}

// takeFinished claims, in launch order, every finished task owner launched
// that has not been handed over yet.
func (b *backgroundTasks) takeFinished(owner string) []*backgroundTask {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []*backgroundTask
	for _, t := range b.tasks {
		if t.owner == owner && !t.delivered && t.finished() {
			t.delivered = true
			out = append(out, t)
		}
	}
	return out
}

// agentsRunning reports whether owner has a background Agent still running.
func (b *backgroundTasks) agentsRunning(owner string) bool {
	if b == nil {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, t := range b.tasks {
		if t.owner == owner && t.agent && !t.finished() {
			return true
		}
	}
	return false
}

// awaitAfterTurn is what a `claude -p` session does once a turn has ended and
// Stop let it: it returns the next finished task of owner to hand over as a
// new turn, waiting while owner still has a background Agent running. It
// returns nil when there is nothing left to wait for.
func (b *backgroundTasks) awaitAfterTurn(ctx context.Context, owner string) *backgroundTask {
	for {
		if ts := b.takeFirstFinished(owner); ts != nil {
			return ts
		}
		if !b.agentsRunning(owner) {
			return nil
		}
		select {
		case <-b.changed:
		case <-ctx.Done():
			return nil
		}
	}
}

func (b *backgroundTasks) takeFirstFinished(owner string) *backgroundTask {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, t := range b.tasks {
		if t.owner == owner && !t.delivered && t.finished() {
			t.delivered = true
			return t
		}
	}
	return nil
}

// notification is the <task-notification> text for a finished task.
func (t *backgroundTask) notification() string {
	var b strings.Builder
	b.WriteString("<task-notification>\n<task-id>" + t.id + "</task-id>\n")
	if t.toolUseID != "" {
		b.WriteString("<tool-use-id>" + t.toolUseID + "</tool-use-id>\n")
	}
	b.WriteString("<output-file>" + t.outputFile + "</output-file>\n")
	b.WriteString("<status>" + t.status() + "</status>\n")
	b.WriteString("<summary>" + t.summary() + "</summary>")
	if t.agent {
		b.WriteString("\n<note>" + agentNotificationNote + "</note>")
		if t.result != "" {
			b.WriteString("\n<result>" + t.result + "</result>")
		}
		if t.failure == "" {
			fmt.Fprintf(&b, "\n<usage><subagent_tokens>0</subagent_tokens><tool_uses>%d</tool_uses><duration_ms>%d</duration_ms></usage>", t.toolUses, t.durationMs)
		}
	}
	b.WriteString("\n</task-notification>")
	return b.String()
}

func (t *backgroundTask) status() string {
	switch {
	case t.killed.Load():
		return "stopped"
	case t.agent && t.failure != "":
		return "failed"
	case !t.agent && t.exitCode != 0:
		return "failed"
	}
	return "completed"
}

// summary is the notification's one-line summary, in the wording of the
// 2.1.282 binary's notification builders.
func (t *backgroundTask) summary() string {
	if t.agent {
		if t.failure != "" {
			return `Agent "` + t.description + `" failed: ` + t.failure
		}
		return `Agent "` + t.description + `" finished`
	}
	if t.exitCode != 0 {
		return fmt.Sprintf("Background command %q failed with exit code %d", t.description, t.exitCode)
	}
	return fmt.Sprintf("Background command %q completed (exit code %d)", t.description, t.exitCode)
}

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
	writeFrame(cfg, map[string]any{
		"type": "system", "subtype": "task_updated", "task_id": t.id,
		"patch": map[string]any{"status": updated, "end_time": time.Now().UnixMilli()},
	})
	writeFrame(cfg, map[string]any{
		"type": "system", "subtype": "task_notification", "task_id": t.id, "tool_use_id": t.toolUseID,
		"status": t.status(), "output_file": t.outputFile, "summary": summary,
	})
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

// stopOwned ends what owner still has running when its run gives its final
// response: background commands are killed, and each killed one's stopped
// notification goes to the stream only (a `claude -p` session writes none to
// the transcript). It waits for them to be gone.
func (b *backgroundTasks) stopOwned(cfg Config) {
	if b == nil {
		return
	}
	b.mu.Lock()
	var victims []*backgroundTask
	for _, t := range b.tasks {
		if t.owner == cfg.AgentID && !t.agent && !t.finished() {
			t.killed.Store(true)
			t.delivered = true
			victims = append(victims, t)
		}
	}
	b.mu.Unlock()
	for _, t := range victims {
		killGroup(t.cmd)
		<-t.done
	}
}

// shutdown ends the whole session's background work when the run returns: a
// mock run is the session, and nothing outlives it. It kills every command's
// process group, cancels running sub-agents, and waits for all of it.
func (b *backgroundTasks) shutdown() {
	if b == nil {
		return
	}
	b.mu.Lock()
	var cmds []*exec.Cmd
	for _, t := range b.tasks {
		if !t.agent && !t.finished() {
			t.killed.Store(true)
			cmds = append(cmds, t.cmd)
		}
	}
	b.mu.Unlock()
	for _, c := range cmds {
		killGroup(c)
	}
	b.cancel()
	b.wg.Wait()
}

// killGroup kills a background command's whole process group.
func killGroup(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		_ = cmd.Process.Kill()
	}
}

// writeStreamLine writes one line to the output stream in a single Write, so
// a frame another goroutine writes (a background sub-agent's) can never land
// between a line and its newline.
func writeStreamLine(cfg Config, line []byte) {
	if len(line) == 0 {
		return
	}
	buf := make([]byte, 0, len(line)+1)
	buf = append(append(buf, line...), '\n')
	cfg.Out.Write(buf) //nolint:errcheck
}

// inputValidationError is the tool_result real Claude Code returns for a call
// missing required parameters: "<tool_use_error>InputValidationError: <Tool>
// failed due to the following issue(s):\nThe required parameter `x` is
// missing</tool_use_error>", with the zod issues as toolUseResult.
func inputValidationError(tool string, missing []string) toolexec.Result {
	noun := "issue"
	if len(missing) > 1 {
		noun = "issues"
	}
	var lines []string
	var issues []map[string]any
	for _, m := range missing {
		lines = append(lines, "The required parameter `"+m+"` is missing")
		issues = append(issues, map[string]any{
			"expected": "string", "code": "invalid_type", "path": []string{m},
			"message": "Invalid input: expected string, received undefined",
		})
	}
	zod, _ := json.MarshalIndent(issues, "", "  ")
	return toolexec.Result{
		Output:        "<tool_use_error>InputValidationError: " + tool + " failed due to the following " + noun + ":\n" + strings.Join(lines, "\n") + "</tool_use_error>",
		IsError:       true,
		ToolUseResult: "InputValidationError: " + string(zod),
	}
}

// randomID is n lowercase alphanumerics, the shape of a real background task id.
func randomID(n int) string {
	const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789"
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return strings.Repeat("0", n)
	}
	for i := range buf {
		buf[i] = alphabet[int(buf[i])%len(alphabet)]
	}
	return string(buf)
}
