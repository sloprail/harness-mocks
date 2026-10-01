package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	corehooks "github.com/sloprail/harness-mocks/internal/hooks"

	"github.com/sloprail/harness-mocks/claude-mock/internal/childenv"
	"github.com/sloprail/harness-mocks/internal/procexec"
)

func (inv *Invoker) invokeCommand(ctx context.Context, h HandlerSpec, ev EventName, hookCwd string, payload []byte) (HandlerRun, error) {
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
	// CLAUDE_CODE_ENTRYPOINT=sdk-cli are set unconditionally: the real CLI stamps both
	// on every session (both confirmed present in a live session), and a tool the
	// hook shells to that detects "am I under a harness" keys off them — sr-agent's
	// harness detection recognises Claude Code by exactly these two and REFUSES with
	// ErrNoHarness when neither is present. The mock stands in for Claude Code, so it
	// must present them whether or not a session id is known. CLAUDE_CODE_SESSION_ID
	// is set only when non-empty (the real CLI carries the active session id in each
	// hook's env; a tool reads it to resolve "the current session").
	// sr:docs https://code.claude.com/docs/en/env-vars (CLAUDECODE, CLAUDE_CODE_ENTRYPOINT, CLAUDE_CODE_SESSION_ID)
	cmd.Env = procexec.Env(os.Environ(), childenv.Identity(inv.sessionID), childenv.Defaults())
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

	// Claude Code reads the JSON on every exit code, not only 0 (docs, "Exit
	// code output"): parse it before the status decides anything.
	if stdout.Len() > 0 {
		switch {
		case !corehooks.IsJSONOutput(stdout.String()):
			run.Output.PlainText = strings.TrimSpace(stdout.String())
		case !json.Valid(stdout.Bytes()):
			run.JSONError = "Hook output looks like a JSON object but is not valid JSON"
		default:
			if err := json.Unmarshal(stdout.Bytes(), &run.Output); err != nil {
				run.JSONError = "Hook JSON output validation failed — " + err.Error()
			} else {
				run.JSONParsed = true
			}
		}
	}

	// sr:provides hook-exit-code-semantics/claude
	if corehooks.VerdictOf(exitCode, strictExitEvents[ev]) == corehooks.Blocked {
		run.Blocked = true
		return run, &BlockError{Command: h.Command, Stderr: run.Stderr}
	}
	if runErr != nil {
		slog.Debug("hooks: command non-blocking error", "cmd", h.Command, "err", runErr, "stderr", stderr.String())
	}
	return run, nil
}
