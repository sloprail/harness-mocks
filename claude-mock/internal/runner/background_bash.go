package runner

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/sloprail/harness-mocks/claude-mock/internal/childenv"
	"github.com/sloprail/harness-mocks/claude-mock/internal/toolexec"
	"github.com/sloprail/harness-mocks/internal/procexec"
	"github.com/sloprail/harness-mocks/internal/tasks"
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

// exitTrailer is what Claude Code appends to a background command's output file
// when it ends: how it ended.
func exitTrailer(code int, killed bool) string {
	if killed {
		return "\n[killed]\n"
	}
	return fmt.Sprintf("\n[exited with code %d]\n", code)
}

// launchBash starts a background Bash for owner and returns its receipt. Inside
// a foreground sub-agent (cfg.SyncSubagent) the receipt says the command is
// terminated at the sub-agent's final response, as real Claude Code's does.
//
// sr:provides background-bash/claude
// sr:provides foreground-subagent-bash-ends-with-response/claude
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
	task := tasks.NewTask(tasks.Command, id)
	task.ToolUseID, task.Owner, task.Description, task.Command, task.OutputFile = toolUseID, cfg.AgentID, desc, in.Command, outFile
	task.Meta = taskStart{ID: id, ToolUseID: toolUseID, Description: desc, TaskType: "local_bash", Backgrounded: true}
	frames := frameObserver{cfg}
	if err := b.StartCommand(task, tasks.CommandSpec{
		Argv: []string{"/bin/sh", "-c", toolexec.WithSessionEnv(cfg.SessionID, in.Command)}, Dir: cfg.Cwd,
		Env: procexec.Env(os.Environ(), childenv.Tool(cfg.SessionID), childenv.Defaults()),
		Out: out, Trailer: exitTrailer,
		Started: func(t *tasks.Task) { tasks.Announce(b.Registry, t, frames) },
		Ended:   func(t *tasks.Task) { tasks.Conclude(b.Registry, t, frames) },
	}); err != nil {
		return toolexec.Result{Output: fmt.Sprintf("Bash: %v", err), IsError: true}
	}

	if awaitReceipt(task) {
		b.endedAtLaunch.Store(task, true)
	}
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
