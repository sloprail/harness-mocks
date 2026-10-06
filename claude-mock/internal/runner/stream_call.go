package runner

import (
	"context"
	"time"

	"github.com/sloprail/harness-mocks/claude-mock/internal/hooks"
	"github.com/sloprail/harness-mocks/claude-mock/internal/toolexec"
	"github.com/sloprail/harness-mocks/internal/tasks"
)

// runCall carries out the call sc.pending names: it answers a refused one, or executes the tool, gives
// its result and fires its post hooks.
func runCall(ctx context.Context, cfg Config, inv *hooks.Invoker, tr *transcript, bg *backgroundTasks, sc scanResult) (turnResult, error) {
	pending := sc.pending
	defer cfg.steps.finished() // the call's result is given: what another agent's gate may wait for
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
	var startAgent func(answered <-chan struct{}) <-chan struct{}
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
		res = toolexec.Execute(toolexec.WithAgent(ctx, cfg.AgentID), pending.ToolName, pending.ToolInput, cfg.Cwd, cfg.SessionID)
		owned(res)
	}

	// Synthesise and emit the tool_result user record.
	// sr:docs https://docs.anthropic.com/en/docs/claude-code/sdk#stream-json-output-format
	took := time.Since(toolStarted)
	if cfg.HookEvents { // its hook frames come ahead of the result frame, its records after the result's (recorded: runs/include-hook-events)
		firePostTool(ctx, cfg, inv.WithRecorder(tr.holdHookRuns), pending, res, took)
	}
	answered := make(chan struct{})
	var began <-chan struct{}
	if startAgent != nil { // a background agent's start is announced ahead of the result that answers its launch
		began = startAgent(answered)
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
	if began != nil {
		select { // and its SubagentStart fires before the next call of the message is taken
		case <-began:
		case <-ctx.Done():
		}
	}

	// A background task that finished while this tool ran is handed over now,
	// inside the turn.
	bg.deliverMidTurn(ctx, cfg, inv, tr)

	// A sub-agent at its maxTurns ends here, with no result frame.
	atLimit := cfg.TurnLimit.Step()
	return turnResult{sig: pending.ToolName + ":" + string(pending.ToolInput), lastText: sc.lastText, done: atLimit}, nil
}
