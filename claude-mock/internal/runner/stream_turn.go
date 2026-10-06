package runner

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/sloprail/harness-mocks/claude-mock/internal/hooks"
	"github.com/sloprail/harness-mocks/claude-mock/internal/toolexec"
	"github.com/sloprail/harness-mocks/internal/tasks"
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
	if pending.ToolName != "" { // the call's result is given when this turn is over: what another agent's gate may wait for
		defer cfg.steps.finished()
	}
	if cfg.AgentID == "" && (pending.ToolName != "" || sc.lastText != "") {
		bg.run.turn()
	}
	if pending.ToolName == "" {
		if sc.done || sc.compactSig == "" {
			return turnResult{done: true, lastText: sc.lastText, resultLine: sc.resultLine}, nil
		}
		// The invocation compacted the context and stopped: the turn goes on
		// after a compaction, so the script runs again.
		return turnResult{sig: sc.compactSig, lastText: sc.lastText}, nil
	}

	if res, refused, err := answerRefusedCall(ctx, cfg, inv, tr, bg, sc); refused || err != nil {
		return res, err
	}

	if err := refuseBackgroundHookFrames(cfg, inv, pending); err != nil {
		return turnResult{}, err
	}
	if text, denied := ruleDenial(inv, pending); denied {
		return denyByRule(ctx, cfg, inv, tr, bg, pending, text, sc.lastText)
	}

	cfg.steps.holdExec(ctx, sc.execGate) // the script's order of the agents' steps, at the call's carrying out
	// tool_use was seen — execute it.
	//
	// Some tools are special-cased here at the stream layer:
	//   - Agent (alias Task; real Claude renamed Task→Agent in v2.1.63) spawns a
	//     NESTED subagent run rather than a pure FS/Bash op, so it needs ctx, cfg,
	//     inv and the session file (none of which toolexec.Execute has access to).
	//     With run_in_background it runs concurrently (background.go).
	//   - Bash with run_in_background starts a background command.
	//   - ScheduleWakeup is routed to runScheduleWakeupTool purely for arg
	//     validation; on success it just returns a success tool_result. There is no
	//     real delay in the mock — the turn loop ALREADY re-runs the script after
	//     every tool_use, which IS the "wake-up fired, resume" behaviour.
	// sr:docs https://code.claude.com/docs/en/sub-agents
	var res toolexec.Result
	var startAgent func(answered <-chan struct{})
	toolStarted := time.Now()
	switch {
	case pending.ToolName == "SendMessage":
		res, startAgent = bg.resumeAgent(cfg, inv, pending.ToolUseID, pending.ToolInput)
	case isAgentTool(pending.ToolName) && agentRunsInBackground(cfg, pending.ToolInput):
		res, startAgent = bg.launchAgent(cfg, inv, pending.ToolUseID, pending.ToolInput, tr)
	case isAgentTool(pending.ToolName):
		res = runAgentTool(ctx, cfg, inv, pending.ToolUseID, pending.ToolInput, tr)
	case isScheduleWakeupTool(pending.ToolName):
		res = runScheduleWakeupTool(cfg.wake, pending.ToolInput)
	case pending.ToolName == "Bash" && tasks.RunsInBackground(runsInBackground(pending.ToolInput), cfg.BackgroundTasksDisabled):
		res = bg.launchBash(cfg, pending.ToolUseID, pending.ToolInput)
	default:
		// cfg.SessionID is the session the Bash tool exports as CLAUDE_CODE_SESSION_ID.
		// A subagent's nested run carries the PARENT's session id (subagentRun.run), the
		// same id its hooks get — real claude shares one session_id across subagents.
		owned := ownedBashFrames(cfg, pending)
		res = toolexec.Execute(ctx, pending.ToolName, pending.ToolInput, cfg.Cwd, cfg.SessionID)
		owned(res)
	}

	// Synthesise and emit the tool_result user record.
	// sr:docs https://docs.anthropic.com/en/docs/claude-code/sdk#stream-json-output-format
	took := time.Since(toolStarted)
	if cfg.HookEvents { // its hook frames come ahead of the result frame, its records after the result's (recorded: runs/include-hook-events)
		firePostTool(ctx, cfg, inv.WithRecorder(tr.holdHookRuns), pending, res, took)
	}
	answered := make(chan struct{})
	if startAgent != nil { // a background agent's start is announced ahead of the result that answers its launch
		startAgent(answered)
	}
	if err := emitToolResult(cfg, pending, res, tr); err != nil {
		close(answered)
		return turnResult{}, err
	}
	tr.flushHookRuns()
	if m, ok := res.ToolUseResult.(map[string]any); ok && isAgentTool(pending.ToolName) {
		if id, _ := m["agentId"].(string); id != "" {
			cfg.steps.endChild(id)
		}
	}

	// PostToolUse for the synthesised result; PostToolUseFailure instead when
	// the tool ran and failed (a Bash exiting non-zero, a file tool's error),
	// with the text the agent got as error (recorded: snapshots/runs/bashfail,
	// tool-errors). tool_response is the tool's structured result where it has
	// one, else the text the agent got. Input the tool could not take fires
	// neither: the tool never ran. Neither leaves a record here.
	// sr:docs https://code.claude.com/docs/en/hooks#posttoolusefailure
	if !cfg.HookEvents {
		firePostTool(ctx, cfg, inv, pending, res, took)
	}
	close(answered) // the sub-agent's run begins now

	// A background task that finished while this tool ran is handed over now,
	// inside the turn.
	bg.deliverMidTurn(ctx, cfg, inv, tr)

	// A sub-agent at its maxTurns ends here, with no result frame.
	atLimit := cfg.TurnLimit.Step()
	return turnResult{sig: pending.ToolName + ":" + string(pending.ToolInput), lastText: sc.lastText, done: atLimit}, nil
}
