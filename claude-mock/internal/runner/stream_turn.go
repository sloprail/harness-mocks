package runner

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/sloprail/harness-mocks/claude-mock/internal/hooks"
	"github.com/sloprail/harness-mocks/claude-mock/internal/toolexec"
)

// runOneTurnSig executes the script once, processes its JSONL output, and
// executes the tool call it ended on, if any.
func runOneTurnSig(ctx context.Context, cfg Config, inv *hooks.Invoker, tr *transcript, bg *backgroundTasks) (turnResult, error) {
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
	if pending.ToolName == "" {
		if sc.done || sc.compactSig == "" {
			return turnResult{done: true, lastText: sc.lastText, resultLine: sc.resultLine}, nil
		}
		// The invocation compacted the context and stopped: the turn goes on
		// after a compaction, so the script runs again.
		return turnResult{sig: sc.compactSig, lastText: sc.lastText}, nil
	}

	// Input the tool cannot take: its tool_use_error is the result, and the
	// turn goes on; no hook fires (recorded: snapshots/runs/tool-invalid-input).
	if pending.Invalid != nil {
		if err := emitToolResult(cfg, pending, *pending.Invalid, tr); err != nil {
			return turnResult{}, err
		}
		return turnResult{sig: "invalid:" + pending.ToolName + ":" + string(pending.ToolInput), lastText: sc.lastText}, nil
	}

	// PreToolUse REFUSED this tool call — an exit-0 permissionDecision deny, or
	// an exit 2. The tool does not run and no PostToolUse fires; the refusal is
	// the tool_result, "PreToolUse:<Tool> hook error: <reason>" (for an exit 2,
	// "[<command>]: <stderr>" as the reason), and the turn goes on: the script
	// runs again and reads it. Claude 2.1.282 did exactly this for both forms in
	// a controlled run. The loop guard signature is the blocked tool_use, so an
	// agent that re-emits the identical blocked call is still bounded.
	// sr:docs https://code.claude.com/docs/en/hooks#pretooluse
	if pending.Blocked {
		text := "PreToolUse:" + pending.ToolName + " hook error: " + pending.BlockReason
		blockRes := toolexec.Result{Output: text, IsError: true, ToolUseResult: "Error: " + text}
		if err := emitToolResult(cfg, pending, blockRes, tr); err != nil {
			return turnResult{}, err
		}
		bg.deliverMidTurn(ctx, cfg, inv, tr)
		return turnResult{sig: "blocked:" + pending.ToolName + ":" + string(pending.ToolInput), lastText: sc.lastText}, nil
	}

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
	var startAgent func()
	toolStarted := time.Now()
	switch {
	case isAgentTool(pending.ToolName) && runsInBackground(pending.ToolInput):
		res, startAgent = bg.launchAgent(cfg, inv, pending.ToolUseID, pending.ToolInput, tr)
	case isAgentTool(pending.ToolName):
		res = runAgentTool(ctx, cfg, inv, pending.ToolUseID, pending.ToolInput, tr)
	case isScheduleWakeupTool(pending.ToolName):
		res = runScheduleWakeupTool(cfg.wake, pending.ToolInput)
	case pending.ToolName == "Bash" && runsInBackground(pending.ToolInput):
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
	if err := emitToolResult(cfg, pending, res, tr); err != nil {
		return turnResult{}, err
	}

	// PostToolUse for the synthesised result; PostToolUseFailure instead when
	// the tool ran and failed (a Bash exiting non-zero, a file tool's error),
	// with the text the agent got as error (recorded: snapshots/runs/bashfail,
	// tool-errors). tool_response is the tool's structured result where it has
	// one, else the text the agent got. Input the tool could not take fires
	// neither: the tool never ran. Neither leaves a record here.
	// sr:docs https://code.claude.com/docs/en/hooks#posttoolusefailure
	took := time.Since(toolStarted).Milliseconds()
	firePostTool(ctx, cfg, inv, pending, res, took)
	if startAgent != nil {
		startAgent()
	}

	// A background task that finished while this tool ran is handed over now,
	// inside the turn.
	bg.deliverMidTurn(ctx, cfg, inv, tr)

	return turnResult{sig: pending.ToolName + ":" + string(pending.ToolInput), lastText: sc.lastText}, nil
}
