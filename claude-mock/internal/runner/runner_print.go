package runner

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/sloprail/harness-mocks/claude-mock/internal/hooks"
)

// runPrintMode runs the supervisor script once with raw stdout capture (no JSONL
// parsing, no session persistence). The script's working directory is cfg.Cwd
// (set to the session dir by the supervisor caller). Raw stdout is written to
// cfg.Out. UserPromptSubmit fires before it and Stop after it, with the output
// as last_assistant_message; a Stop block is returned as the run's error, so
// the caller sees the validation failure.
//
// sr:docs https://code.claude.com/docs/en/cli-reference#--print
func runPrintMode(ctx context.Context, cfg Config, inv *hooks.Invoker, tr *transcript) error {
	// Fire UserPromptSubmit so any hook in the project-dir settings can intercept
	// even in print mode. The hook cannot replace the prompt (real Claude
	// contract); it may only append additionalContext or block (→ Fire errors).
	if cfg.Prompt != "" {
		promptOut, err := inv.Fire(ctx, hooks.Input{
			SessionID:     cfg.SessionID,
			Cwd:           cfg.Cwd,
			HookEventName: hooks.EventUserPromptSubmit,
			Prompt:        cfg.Prompt,
		})
		if err != nil {
			return fmt.Errorf("claude-mock: UserPromptSubmit hook blocked: %w", err)
		}
		cfg.AdditionalContext = additionalContextFrom(promptOut)
	}

	// Opt-in: drive the script through the streaming turn loop so it can emit
	// tool_use records (e.g. an Agent/Task tool_use → nested sub-agent). The
	// turn loop fires Stop itself.
	if os.Getenv("A10N_MOCK_PRINT_STREAM") == "1" {
		return streamAndHook(ctx, cfg, inv, tr)
	}

	var captured bytes.Buffer
	cmd := exec.CommandContext(ctx, "/bin/sh", cfg.ScriptPath) //nolint:gosec
	cmd.Dir = cfg.Cwd                                          // supervisor writes memory files here
	cmd.Env = buildEnv(cfg, tr)
	cmd.Stderr = cfg.Stderr
	cmd.Stdout = io.MultiWriter(cfg.Out, &captured) // raw text passthrough — no JSONL parsing
	runErr := cmd.Run()

	// In print mode the Stop hook validates response.json (exit 2 = validation
	// failure). Propagate a block as an error so RunSupervisor knows the run failed.
	active := false
	last := strings.TrimSpace(captured.String())
	tasks := []hooks.BackgroundTask{}
	crons := []any{}
	stopOut, stopErr := inv.Fire(ctx, hooks.Input{
		SessionID:            cfg.SessionID,
		Cwd:                  cfg.Cwd,
		HookEventName:        hooks.EventStop,
		StopHookActive:       &active,
		LastAssistantMessage: &last,
		BackgroundTasks:      &tasks,
		SessionCrons:         &crons,
	})
	if runErr != nil {
		return runErr
	}
	if stopErr == nil && stopOut.Decision == "block" {
		stopErr = fmt.Errorf("claude-mock: Stop hook blocked: %s", stopOut.Reason)
	}
	return stopErr
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// lockedWriter serialises writes to the session's output stream.
type lockedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}
