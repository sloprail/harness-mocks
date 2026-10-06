package runner

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/sloprail/harness-mocks/claude-mock/internal/hooks"
)

// runOneTurnSig executes the script once, processes its JSONL output, and
// executes the tool call it ended on, if any.
func runOneTurnSig(ctx context.Context, cfg Config, inv *hooks.Invoker, tr *transcript, bg *backgroundTasks) (turnResult, error) {
	if err := bg.refused.Err(); err != nil { // a sub-agent's script asked for what the mock does not implement
		return turnResult{}, err
	}
	cmd := exec.CommandContext(ctx, "/bin/sh", cfg.ScriptPath) //nolint:gosec
	cmd.Dir = cfg.Cwd
	cmd.Env = buildEnv(cfg, tr)
	cmd.Stderr = cfg.Stderr

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return turnResult{}, fmt.Errorf("claude-mock: stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return turnResult{}, fmt.Errorf("claude-mock: start script: %w", err)
	}

	sc, scanErr := scanLines(ctx, stdoutPipe, cfg, inv, tr)
	waitErr := cmd.Wait()

	if scanErr != nil {
		return turnResult{}, scanErr
	}
	if waitErr != nil {
		return turnResult{}, waitErr
	}
	pending := sc.pending
	if cfg.AgentID == "" && len(sc.group) > 0 { // each call of a message counts as a turn (recorded: runs/bgagent-concurrent-limit)
		bg.run.addTurns(len(sc.group))
	} else if cfg.AgentID == "" && sc.lastText != "" {
		bg.run.turn()
	}
	if pending.ToolName == "" {
		if sc.done || sc.compactSig == "" {
			return turnResult{done: true, lastText: sc.lastText, resultLine: sc.resultLine, emptyReply: sc.thinking && sc.lastText == ""}, nil
		}
		// The invocation compacted the context and stopped: the turn goes on
		// after a compaction, so the script runs again.
		return turnResult{sig: sc.compactSig, lastText: sc.lastText}, nil
	}
	// The calls of a message are carried out one after the other, each answered in turn; the turn's
	// result is the last one's (recorded: runs/bgagent-concurrent-limit: two Agent calls in one message).
	for i := range sc.group {
		sc.pending = sc.group[i]
		res, err := runCall(ctx, cfg, inv, tr, bg, sc)
		if err != nil || i == len(sc.group)-1 {
			return res, err
		}
	}
	return turnResult{}, nil
}
