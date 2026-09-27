package runner

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/sloprail/harness-mocks/claude-mock/internal/hooks"
	"github.com/sloprail/harness-mocks/claude-mock/internal/toolexec"
)

// Background tasks: a Bash or an Agent called with run_in_background, the
// receipts real Claude Code answers them with, TaskOutput reading one back,
// and the <task-notification> turn that hands a finished one to the agent.
//
// The text and fields are taken from real transcripts:
//
//   - A background Bash is answered at once with
//     "Command running in background with ID: <id>. Output is being written
//     to: <file>. You will be notified when it completes. …", and its record
//     carries toolUseResult.backgroundTaskId.
//   - A background Agent is answered with "Async agent launched successfully.
//     … agentId: <id> (internal ID - do not mention to user. …)", and its
//     record carries toolUseResult {isAsync, status: "async_launched", agentId}.
//   - When a task finishes, a user record whose content is a
//     <task-notification> block (task-id, tool-use-id, output-file, status,
//     summary) is written, with origin {kind: "task-notification"}, and the
//     agent runs again to read it.
//
// What the mock does NOT reproduce: real concurrency with the agent's own
// turns. A background Bash really runs in the background, but a background
// Agent runs to completion before its receipt is returned — the sub-agent's
// records and hooks all happen, in its own sidechain file, just not
// interleaved with the parent's. Its notification is still delivered only
// after the receipt, at the next turn boundary, as the real one is.

// maxBackgroundWait bounds how long the end of a turn waits for a background
// task still running, before handing the agent its notification. A scenario's
// background command is a test fixture, not real work.
const maxBackgroundWait = 30 * time.Second

type backgroundTask struct {
	id          string
	toolUseID   string
	agent       bool
	description string
	outputFile  string

	done      chan struct{}
	exitCode  int
	result    string // an agent's final text
	delivered bool
}

type backgroundTasks struct {
	cfg   Config
	tr    *transcript
	mu    sync.Mutex
	tasks []*backgroundTask
	procs map[*backgroundTask]*exec.Cmd
}

func newBackgroundTasks(cfg Config, tr *transcript) *backgroundTasks {
	return &backgroundTasks{cfg: cfg, tr: tr, procs: map[*backgroundTask]*exec.Cmd{}}
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

// tasksDir is where a background task's output file lives — the per-session
// tasks directory real Claude Code writes under the system temp dir.
func (b *backgroundTasks) tasksDir() string {
	encoded := nonAlphanumRe.ReplaceAllString(resolveEncodingCwd(b.cfg.Cwd), "-")
	return filepath.Join(os.TempDir(), "claude-mock", encoded, b.cfg.SessionID, "tasks")
}

// launchBash starts a background Bash and returns its receipt.
func (b *backgroundTasks) launchBash(ctx context.Context, toolUseID string, raw json.RawMessage) toolexec.Result {
	var in struct {
		Command     string `json:"command"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(raw, &in); err != nil || in.Command == "" {
		return toolexec.Result{Output: "Bash: missing or invalid 'command' field", IsError: true}
	}
	id := "b" + randomID(8)
	dir := b.tasksDir()
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
	task := &backgroundTask{id: id, toolUseID: toolUseID, description: desc, outputFile: outFile, done: make(chan struct{})}

	// Not bound to ctx: a background command outlives the turn that started it.
	cmd := exec.Command("/bin/sh", "-c", in.Command) //nolint:gosec
	cmd.Dir = b.cfg.Cwd
	cmd.Env = append(os.Environ(), "CLAUDE_CODE_SESSION_ID="+b.cfg.SessionID)
	cmd.Stdout = out
	cmd.Stderr = out
	if err := cmd.Start(); err != nil {
		out.Close()
		return toolexec.Result{Output: fmt.Sprintf("Bash: %v", err), IsError: true}
	}
	b.mu.Lock()
	b.tasks = append(b.tasks, task)
	b.procs[task] = cmd
	b.mu.Unlock()
	go func() {
		err := cmd.Wait()
		out.Close()
		code := 0
		if err != nil {
			code = 1
			if cmd.ProcessState != nil {
				code = cmd.ProcessState.ExitCode()
			}
		}
		task.exitCode = code
		close(task.done)
	}()

	hint := "Session cwd remains " + b.cfg.Cwd + "; directory changes made by the backgrounded command do not apply to subsequent commands."
	return toolexec.Result{
		Output: "Command running in background with ID: " + id + ". Output is being written to: " + outFile +
			". You will be notified when it completes. To check interim output, use Read on that file path.\n" + hint,
		ToolUseResult: map[string]any{
			"stdout": "", "stderr": "", "interrupted": false, "isImage": false, "noOutputExpected": false,
			"backgroundTaskId": id, "backgroundCwdHint": hint,
		},
	}
}

// launchAgent runs a background Agent and returns its receipt. See the file
// comment on why the sub-agent runs to completion first.
func (b *backgroundTasks) launchAgent(ctx context.Context, inv *hooks.Invoker, toolUseID string, raw json.RawMessage) toolexec.Result {
	var in struct {
		Description string `json:"description"`
		Prompt      string `json:"prompt"`
	}
	_ = json.Unmarshal(raw, &in)
	agentID, err := newAgentID()
	if err != nil {
		return toolexec.Result{Output: fmt.Sprintf("Agent: generate agent_id: %v", err), IsError: true}
	}
	dir := b.tasksDir()
	_ = os.MkdirAll(dir, 0o755)
	outFile := filepath.Join(dir, agentID+".output")

	res, _ := runAgentToolFor(ctx, b.cfg, inv, toolUseID, raw, b.tr, agentID)
	task := &backgroundTask{
		id: agentID, toolUseID: toolUseID, agent: true, description: in.Description,
		outputFile: outFile, done: make(chan struct{}), result: agentFinalText(res.Output),
	}
	if res.IsError {
		task.exitCode = 1
	}
	close(task.done)
	b.mu.Lock()
	b.tasks = append(b.tasks, task)
	b.mu.Unlock()

	return toolexec.Result{
		Output: "Async agent launched successfully. (This tool result is internal metadata — never quote or paste any part of it, including the agentId below, into a user-facing reply.)\n" +
			"agentId: " + agentID + " (internal ID - do not mention to user. Use SendMessage with to: '" + agentID + "', summary: '<5-10 word recap>' to continue this agent.)\n" +
			"The agent is working in the background. You will be notified automatically when it completes.\n" +
			"output_file: " + outFile,
		ToolUseResult: map[string]any{
			"isAsync": true, "status": "async_launched", "agentId": agentID,
			"description": in.Description, "prompt": in.Prompt, "outputFile": outFile,
		},
	}
}

// agentFinalText strips the agentId/agentType header buildAgentResultContent
// puts on a foreground Agent's result, leaving the agent's reply.
func agentFinalText(out string) string {
	lines := strings.SplitN(out, "\n", 3)
	if len(lines) == 3 && strings.HasPrefix(lines[0], "agentId: ") && strings.HasPrefix(lines[1], "agentType: ") {
		return lines[2]
	}
	return out
}

// output answers TaskOutput: the task's output once it has finished (or at
// once, with block=false), in the tagged shape the tool returns.
func (b *backgroundTasks) output(ctx context.Context, raw json.RawMessage) toolexec.Result {
	var in struct {
		TaskID  string `json:"task_id"`
		BashID  string `json:"bash_id"`
		Block   any    `json:"block"`
		Timeout int    `json:"timeout"`
	}
	_ = json.Unmarshal(raw, &in)
	id := in.TaskID
	if id == "" {
		id = in.BashID
	}
	task := b.find(id)
	if task == nil {
		return toolexec.Result{Output: "No task found with ID: " + id, IsError: true}
	}
	block := true
	if v, ok := in.Block.(bool); ok {
		block = v
	} else if v, ok := in.Block.(string); ok && v == "false" {
		block = false
	}
	status := "completed"
	if block {
		wait := maxBackgroundWait
		if in.Timeout > 0 {
			wait = time.Duration(in.Timeout) * time.Millisecond
		}
		select {
		case <-task.done:
		case <-time.After(wait):
			status = "running"
		case <-ctx.Done():
			status = "running"
		}
	} else {
		select {
		case <-task.done:
		default:
			status = "running"
		}
	}
	kind, output := "local_bash", ""
	if task.agent {
		kind, output = "local_agent", task.result
	} else if data, err := os.ReadFile(task.outputFile); err == nil {
		output = strings.TrimRight(string(data), "\n")
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "<retrieval_status>%s</retrieval_status>\n\n", map[bool]string{true: "success", false: "timeout"}[status == "completed"])
	fmt.Fprintf(&sb, "<task_id>%s</task_id>\n\n<task_type>%s</task_type>\n\n<status>%s</status>\n\n", task.id, kind, status)
	if status == "completed" && !task.agent {
		fmt.Fprintf(&sb, "<exit_code>%d</exit_code>\n\n", task.exitCode)
	}
	fmt.Fprintf(&sb, "<output>\n%s\n</output>", output)
	return toolexec.Result{Output: sb.String()}
}

func (b *backgroundTasks) find(id string) *backgroundTask {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, t := range b.tasks {
		if t.id == id {
			return t
		}
	}
	return nil
}

// deliverFinished writes a <task-notification> turn for every task that has
// finished and not been announced, in launch order, and reports whether it
// wrote any. With wait set (the end of a turn) it first waits, bounded, for
// tasks still running — the agent is not finished while its background work
// is, since that work will wake it.
func (b *backgroundTasks) deliverFinished(wait bool) bool {
	if b == nil {
		return false
	}
	b.mu.Lock()
	tasks := append([]*backgroundTask(nil), b.tasks...)
	b.mu.Unlock()
	delivered := false
	for _, t := range tasks {
		if t.delivered {
			continue
		}
		if wait {
			select {
			case <-t.done:
			case <-time.After(maxBackgroundWait):
				continue
			}
		} else {
			select {
			case <-t.done:
			default:
				continue
			}
		}
		t.delivered = true
		delivered = true
		b.notify(t)
	}
	return delivered
}

func (b *backgroundTasks) notify(t *backgroundTask) {
	var summary string
	if t.agent {
		summary = fmt.Sprintf("Agent %q completed", t.description)
	} else {
		summary = fmt.Sprintf("Background command %q completed (exit code %d)", t.description, t.exitCode)
	}
	status := "completed"
	if t.exitCode != 0 {
		status = "failed"
	}
	content := "<task-notification>\n<task-id>" + t.id + "</task-id>\n<tool-use-id>" + t.toolUseID +
		"</tool-use-id>\n<output-file>" + t.outputFile + "</output-file>\n<status>" + status +
		"</status>\n<summary>" + summary + "</summary>\n"
	if t.agent {
		content += "<result>" + t.result + "</result>\n"
	}
	content += "</task-notification>"
	rec := map[string]any{
		"type":         "user",
		"message":      map[string]any{"role": "user", "content": content},
		"origin":       map[string]any{"kind": "task-notification"},
		"promptSource": "system",
	}
	line, err := marshalRecord(rec)
	if err != nil {
		return
	}
	b.cfg.Out.Write(line)         //nolint:errcheck
	b.cfg.Out.Write([]byte{'\n'}) //nolint:errcheck
	b.tr.persist(line)
}

// wait stops what is still running when the run ends — a mock run is the
// whole session, and nothing outlives it.
func (b *backgroundTasks) wait() {
	b.mu.Lock()
	defer b.mu.Unlock()
	for t, c := range b.procs {
		select {
		case <-t.done:
		default:
			if c.Process != nil {
				_ = c.Process.Kill()
			}
		}
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
