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
}

// NewInvoker creates an Invoker backed by the given settings. sessionID is
// exported to every command hook as CLAUDE_CODE_SESSION_ID — mirroring the real
// claude CLI, which puts the active session id in each hook's environment (verified
// against claude 2.x: SessionStart/UserPromptSubmit/PreToolUse all see it). Tools a
// hook shells to — e.g. `a10n-task-executor session autopilot` — read it to resolve
// "the current session" without an explicit flag.
//
// a10n:docs https://code.claude.com/docs/en/env-vars (CLAUDE_CODE_SESSION_ID)
func NewInvoker(settings *Settings, cwd, sessionID string) *Invoker {
	return &Invoker{settings: settings, cwd: cwd, sessionID: sessionID}
}

// Fire invokes all handlers configured for the event and matcher, then returns
// the merged Output. Command hooks that exit 2 are treated as blocking errors
// and returned via the error return. Other non-zero exits are non-blocking
// (logged and ignored).
func (inv *Invoker) Fire(ctx context.Context, input Input) (Output, error) {
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
	// (see agent.go's subCwd), and a hook command that shells out to an a10n-* binary
	// (e.g. `a10n-workspace session subagent-stop`) resolves ITS OWN identity from
	// os.Getwd() — so if the subprocess actually ran in the PARENT's cwd regardless of
	// what the payload claimed, any such binary would silently see the parent's tree,
	// not the isolated one, no matter what "cwd" the JSON stdin says. Falls back to
	// inv.cwd only for the (should-never-happen) case of an empty input.Cwd.
	hookCwd := input.Cwd
	if hookCwd == "" {
		hookCwd = inv.cwd
	}

	var merged Output
	for _, h := range handlers {
		out, blockErr := inv.invoke(ctx, h, hookCwd, payload)
		if blockErr != nil {
			return merged, blockErr
		}
		mergeOutput(&merged, out)
	}
	return merged, nil
}

func (inv *Invoker) invoke(ctx context.Context, h HandlerSpec, hookCwd string, payload []byte) (Output, error) {
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
		return inv.invokeHTTP(ctx, h, payload)
	default:
		slog.Debug("hooks: unsupported handler type", "type", h.Type)
		return Output{}, nil
	}
}

func (inv *Invoker) invokeCommand(ctx context.Context, h HandlerSpec, hookCwd string, payload []byte) (Output, error) {
	command := strings.TrimSpace(h.Command)
	if command == "" {
		return Output{}, nil
	}
	// Run command hooks through the shell, exactly as real Claude Code does. The
	// command string is an arbitrary shell line — plugins wrap the script path in
	// double quotes to survive spaces (e.g. "${CLAUDE_PLUGIN_ROOT}/hooks/x.sh"),
	// pass arguments, or reference env vars resolved at hook-run time. Splitting on
	// whitespace would break all of those, so we delegate parsing to /bin/sh.
	//
	// a10n:docs https://code.claude.com/docs/en/hooks#hook-types (command hooks run in the shell)
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
	// Mirror the real claude CLI: expose the active session id to the hook env.
	cmd.Env = os.Environ()
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

	if exitCode == 2 {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = "hook blocked the action"
		}
		return Output{}, fmt.Errorf("hooks: command blocked: %s", msg)
	}
	if runErr != nil {
		slog.Debug("hooks: command non-blocking error", "cmd", h.Command, "err", runErr, "stderr", stderr.String())
		return Output{}, nil
	}

	var out Output
	if stdout.Len() > 0 {
		if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
			slog.Debug("hooks: command output not valid JSON", "cmd", h.Command, "err", err)
		}
	}
	return out, nil
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
