package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

const defaultHookTimeout = 60 * time.Second

// hookKillGrace bounds how long Run may go on waiting after the process group
// has been killed — the ceiling on a descendant that does not die. It is a
// backstop for an already-abnormal case, so it is short; the kill itself is
// what ends the hook.
const hookKillGrace = 2 * time.Second

// Invoker fires hook handlers for a given event and collects their output.
type Invoker struct {
	settings  *Settings
	cwd       string
	sessionID string

	// transcriptPath is put on every payload that does not name its own. Real
	// Claude Code sends transcript_path on EVERY hook event (it is one of the
	// documented common input fields), including a sub-agent's tool calls, where
	// it is the SESSION's transcript and the sub-agent is named by agent_id.
	// sr:docs https://code.claude.com/docs/en/hooks#common-input-fields
	transcriptPath string

	// recorder, when set, is handed every handler's run — what the harness then
	// writes into the transcript as a hook attachment record.
	recorder func(Input, []HandlerRun)

	agentID, agentType string
}

// SetTranscriptPath sets the transcript_path every payload carries unless the
// caller names one.
func (inv *Invoker) SetTranscriptPath(path string) { inv.transcriptPath = path }

// TranscriptPath is the transcript_path payloads carry by default.
func (inv *Invoker) TranscriptPath() string { return inv.transcriptPath }

// SetAgent makes every payload fired through this invoker carry agent_id and
// agent_type, as real Claude Code does for every hook event fired inside a
// sub-agent. Events that already name an agent keep theirs.
// sr:docs https://code.claude.com/docs/en/hooks#common-input-fields
func (inv *Invoker) SetAgent(agentID, agentType string) {
	inv.agentID, inv.agentType = agentID, agentType
}

// WithRecorder returns a copy of inv that hands its handler runs to fn instead.
func (inv *Invoker) WithRecorder(fn func(Input, []HandlerRun)) *Invoker {
	c := *inv
	c.recorder = fn
	return &c
}

// SetRecorder installs the function every fired event's handler runs are handed
// to, after the handlers have run. See HandlerRun.
func (inv *Invoker) SetRecorder(fn func(Input, []HandlerRun)) { inv.recorder = fn }

// HandlerRun is what one hook handler did, in the terms real Claude Code
// records it in a transcript's hook attachment: its command, its streams, its
// exit code and how long it took. Blocked is an exit 2.
type HandlerRun struct {
	Command    string
	Stdout     string
	Stderr     string
	ExitCode   int
	DurationMs int64
	Blocked    bool
	Output     Output
}

// NewInvoker creates an Invoker backed by the given settings. Every command hook
// runs with three Claude-Code environment variables the real CLI sets on each
// session, so a tool the hook shells to sees the same environment it would under
// real claude:
//
//   - CLAUDE_CODE_SESSION_ID — the active session id (from sessionID here), which
//     a tool like `a10n-task-executor session autopilot` reads to resolve "the
//     current session" without an explicit flag. Verified against claude 2.x:
//     SessionStart/UserPromptSubmit/PreToolUse all see it. Set only when non-empty.
//   - CLAUDECODE=1 and CLAUDE_CODE_ENTRYPOINT=cli — the two variables the real CLI
//     stamps on every session (both confirmed present in a live session). A tool
//     that detects "am I running under a harness" keys off them (sr-agent's harness
//     detection recognises Claude Code by exactly these two and REFUSES with
//     ErrNoHarness when neither is set). The mock STANDS IN FOR Claude Code, so it
//     must present that env unconditionally — see invokeCommand.
//
// sr:docs https://code.claude.com/docs/en/env-vars (CLAUDE_CODE_SESSION_ID, CLAUDECODE, CLAUDE_CODE_ENTRYPOINT)
func NewInvoker(settings *Settings, cwd, sessionID string) *Invoker {
	return &Invoker{settings: settings, cwd: cwd, sessionID: sessionID}
}

// Fire invokes every handler configured for the event and matcher and returns
// their merged Output. Real Claude Code runs all matching hooks of an event
// (docs: "All matching hooks run in parallel"), so a handler that blocks does
// not stop the others from running; the first block is returned, as a
// *BlockError, once they all have.
//
// Exit 2 is a block for every event here; what a block MEANS is the caller's
// to decide, because real Claude Code differs per event — SessionStart and
// SubagentStart treat it as a non-blocking error (docs, "Exit code 2
// behavior per event"), PreToolUse refuses the tool call, Stop re-prompts.
// sr:docs https://code.claude.com/docs/en/hooks#exit-code-2-behavior-per-event
func (inv *Invoker) Fire(ctx context.Context, input Input) (Output, error) {
	if input.TranscriptPath == "" {
		input.TranscriptPath = inv.transcriptPath
	}
	if inv.agentID != "" {
		if input.AgentID == "" {
			input.AgentID = inv.agentID
		}
		if input.AgentID == inv.agentID && input.AgentType == "" {
			input.AgentType = inv.agentType
		}
	}
	handlers := inv.settings.EntriesFor(input.HookEventName, input.ToolName)
	if len(handlers) == 0 {
		return Output{}, nil
	}

	payload, err := json.Marshal(input)
	if err != nil {
		return Output{}, fmt.Errorf("hooks: marshal input: %w", err)
	}

	// The hook subprocess's OWN working directory must be THIS event's cwd, not the
	// Invoker's fixed construction-time cwd: a SubagentStart/Stop fired for a
	// isolation="worktree" subagent carries input.Cwd = the subagent's isolated worktree
	// (see agent.go's subCwd), and a hook command that shells out to a binary resolving
	// its identity from os.Getwd() must see that tree. Falls back to inv.cwd only for
	// an empty input.Cwd.
	hookCwd := input.Cwd
	if hookCwd == "" {
		hookCwd = inv.cwd
	}

	var merged Output
	var runs []HandlerRun
	var firstBlock error
	for _, h := range handlers {
		run, blockErr := inv.invoke(ctx, h, hookCwd, payload)
		runs = append(runs, run)
		if blockErr != nil {
			if firstBlock == nil {
				firstBlock = blockErr
			}
			continue
		}
		mergeOutput(&merged, run.Output)
	}
	if inv.recorder != nil && len(runs) > 0 {
		inv.recorder(input, runs)
	}
	return merged, firstBlock
}

// BlockError is a handler that exited 2: its command and its stderr exactly as
// it wrote it, which real Claude Code quotes as "[<command>]: <stderr>".
type BlockError struct {
	Command string
	Stderr  string
}

func (e *BlockError) Error() string {
	msg := strings.TrimSpace(e.Stderr)
	if msg == "" {
		msg = "hook blocked the action"
	}
	return "hooks: command blocked: " + msg
}

// Quoted is the text real Claude Code shows for an exit-2 block:
// "[<command>]: <stderr>", or "No stderr output" in place of an empty stderr
// (claude 2.1.282, the hook runner's exit-2 branch).
func (e *BlockError) Quoted() string {
	return QuoteBlock(e.Command, e.Stderr)
}

// QuoteBlock renders an exit-2 handler the way real Claude Code quotes it.
func QuoteBlock(command, stderr string) string {
	if stderr == "" {
		stderr = "No stderr output"
	}
	return "[" + command + "]: " + stderr
}

func (inv *Invoker) invoke(ctx context.Context, h HandlerSpec, hookCwd string, payload []byte) (HandlerRun, error) {
	timeout := defaultHookTimeout
	if h.Timeout > 0 {
		timeout = time.Duration(h.Timeout) * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	switch h.Type {
	case "command":
		return inv.invokeCommand(ctx, h, hookCwd, payload)
	case "http":
		out, err := inv.invokeHTTP(ctx, h, payload)
		return HandlerRun{Command: h.URL, Output: out}, err
	default:
		slog.Debug("hooks: unsupported handler type", "type", h.Type)
		return HandlerRun{}, nil
	}
}

func (inv *Invoker) invokeCommand(ctx context.Context, h HandlerSpec, hookCwd string, payload []byte) (HandlerRun, error) {
	command := strings.TrimSpace(h.Command)
	if command == "" {
		return HandlerRun{}, nil
	}
	started := time.Now()
	// Run command hooks through the shell, exactly as real Claude Code does. The
	// command string is an arbitrary shell line — plugins wrap the script path in
	// double quotes to survive spaces (e.g. "${CLAUDE_PLUGIN_ROOT}/hooks/x.sh"),
	// pass arguments, or reference env vars resolved at hook-run time. Splitting on
	// whitespace would break all of those, so we delegate parsing to /bin/sh.
	//
	// sr:docs https://code.claude.com/docs/en/hooks#hook-types (command hooks run in the shell)
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", command) //nolint:gosec
	cmd.Dir = hookCwd

	// The deadline has to reach the hook's DESCENDANTS, not just the shell.
	//
	// A command hook is a shell line, and the interesting ones spawn something:
	// `sr-agent ...`, a model call, a script that backgrounds work. Those are the
	// shell's children. Default CommandContext kills only the direct child, so on
	// expiry the shell died and its children did not — they inherited the stdout
	// and stderr pipes, and cmd.Run() waits for every writer to close them, not
	// for the shell alone. A hook declaring `"timeout": 3` around a backgrounded
	// 25s sleep therefore returned after 25 seconds, and the declared bound did
	// nothing.
	//
	// Setpgid puts the shell in a new process group that its children inherit, so
	// one kill to the negated pgid reaches the whole tree. Cancel does exactly
	// that; returning os.ErrProcessDone keeps an already-finished hook from being
	// reported as killed.
	//
	// WaitDelay is the backstop, not the mechanism: if anything in that group
	// survives the signal (SIGKILL is not catchable, but a process wedged in an
	// uninterruptible syscall can outlast it) it caps how long Run may keep
	// waiting on the inherited pipes. Without it, one unkillable descendant
	// restores the original unbounded hang.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		// Negative pid = "the whole process group". Signalling the group is why
		// the grandchildren die; signalling cmd.Process alone is the bug.
		if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
			if errors.Is(err, syscall.ESRCH) {
				return os.ErrProcessDone
			}
			return err
		}
		return nil
	}
	cmd.WaitDelay = hookKillGrace
	cmd.Stdin = bytes.NewReader(payload)
	// Mirror the real claude CLI's hook environment. CLAUDECODE=1 and
	// CLAUDE_CODE_ENTRYPOINT=cli are set unconditionally: the real CLI stamps both
	// on every session (both confirmed present in a live session), and a tool the
	// hook shells to that detects "am I under a harness" keys off them — sr-agent's
	// harness detection recognises Claude Code by exactly these two and REFUSES with
	// ErrNoHarness when neither is present. The mock stands in for Claude Code, so it
	// must present them whether or not a session id is known. CLAUDE_CODE_SESSION_ID
	// is set only when non-empty (the real CLI carries the active session id in each
	// hook's env; a tool reads it to resolve "the current session").
	// sr:docs https://code.claude.com/docs/en/env-vars (CLAUDECODE, CLAUDE_CODE_ENTRYPOINT, CLAUDE_CODE_SESSION_ID)
	cmd.Env = append(os.Environ(), "CLAUDECODE=1", "CLAUDE_CODE_ENTRYPOINT=cli")
	if inv.sessionID != "" {
		cmd.Env = append(cmd.Env, "CLAUDE_CODE_SESSION_ID="+inv.sessionID)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()
	exitCode := 0
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}
	run := HandlerRun{
		Command:    h.Command,
		Stdout:     stdout.String(),
		Stderr:     stderr.String(),
		ExitCode:   exitCode,
		DurationMs: time.Since(started).Milliseconds(),
	}

	if exitCode == 2 {
		run.Blocked = true
		return run, &BlockError{Command: h.Command, Stderr: run.Stderr}
	}
	if runErr != nil {
		slog.Debug("hooks: command non-blocking error", "cmd", h.Command, "err", runErr, "stderr", stderr.String())
		return run, nil
	}

	if stdout.Len() > 0 {
		if err := json.Unmarshal(stdout.Bytes(), &run.Output); err != nil {
			slog.Debug("hooks: command output not valid JSON", "cmd", h.Command, "err", err)
		}
	}
	return run, nil
}

func (inv *Invoker) invokeHTTP(ctx context.Context, h HandlerSpec, payload []byte) (Output, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.URL, bytes.NewReader(payload))
	if err != nil {
		return Output{}, nil
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		slog.Debug("hooks: http error (non-blocking)", "url", h.URL, "err", err)
		return Output{}, nil
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		slog.Debug("hooks: http non-2xx (non-blocking)", "url", h.URL, "status", resp.StatusCode)
		return Output{}, nil
	}

	var out Output
	if len(body) > 0 {
		if err := json.Unmarshal(body, &out); err != nil {
			slog.Debug("hooks: http response not valid JSON", "url", h.URL, "err", err)
		}
	}
	return out, nil
}

func mergeOutput(dst *Output, src Output) {
	if src.Continue != nil {
		dst.Continue = src.Continue
	}
	if src.StopReason != "" {
		dst.StopReason = src.StopReason
	}
	if src.SystemMessage != "" {
		dst.SystemMessage = src.SystemMessage
	}
	if src.Decision != "" {
		dst.Decision = src.Decision
	}
	if src.Reason != "" {
		dst.Reason = src.Reason
	}
	if src.HookSpecificOutput != nil {
		dst.HookSpecificOutput = src.HookSpecificOutput
	}
}
