package runner

import (
	"encoding/json"
	"fmt"
	"github.com/sloprail/harness-mocks/claude-mock/internal/toolexec"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

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
