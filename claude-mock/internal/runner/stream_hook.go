package runner

import (
	"context"
	"fmt"

	"github.com/sloprail/harness-mocks/claude-mock/internal/hooks"
	corehooks "github.com/sloprail/harness-mocks/internal/hooks"
	"github.com/sloprail/harness-mocks/internal/scenario"
	"github.com/sloprail/harness-mocks/internal/tasks"
	coretools "github.com/sloprail/harness-mocks/internal/tools"
	"github.com/sloprail/harness-mocks/internal/turnloop"
)

// streamAndHook owns a run's turns. At every end of turn (the script's result
// frame) the ROOT run fires Stop — with last_assistant_message and the
// session's still-running background_tasks — whatever background work is
// pending, as real Claude Code does. A Stop block re-prompts the same turn. When
// Stop lets the turn end, a `claude -p` session waits for its background
// AGENTS, and each finished task starts a new turn with its notification (and
// Stop fires again at that turn's end); a background command still running
// when nothing else is left is killed. See background.go.
//
// A nested SUB-AGENT run fires no Stop: the Agent-tool layer (agent.go) owns
// the sub-agent's terminal hook, SubagentStop, and its block→re-run loop. Its
// own background commands end with its final response.
// The run streams one result, at its real end (internal/scenario's Result). Once
// the turn is over a `claude -p` session waits for its background agents, each
// finished task starting a further turn (tasks.NextTurn) until the idle ceiling,
// and ends the background shells that are left after a grace (tasks.ReapAtExit).
//
// sr:provides noninteractive-run/claude
// sr:provides print-waits-for-background-agents/claude
// sr:provides background-bash-reaped-at-exit/claude
// sr:invariant turn-loop
func streamAndHook(ctx context.Context, cfg Config, inv *hooks.Invoker, tr *transcript) error {
	// Every FILE record persisted below goes through tr, which chains it from the
	// one before it (real-CC shape — see sessionWriter). The chain seeds from the
	// last uuid already on disk, so a resume continues the existing chain. The
	// STDOUT stream is written directly with cfg.Out and is NOT chained (it must
	// stay the mock's claude stream).
	if cfg.bg == nil {
		cfg.bg = newBackgroundTasks()
		defer cfg.bg.Shutdown()
	}
	if cfg.wake == nil {
		cfg.wake = coretools.NewWakeups()
	}
	bg := cfg.bg
	nested := cfg.SuppressSubagentHooks
	var lastSig string
	var repeats int
	var stopBlocks int
	var lastText string
	var final scenario.Result // the run's one result frame, held until its turn really ends
	finish := func() { final.Finish(func(line []byte) { writeStreamLine(cfg, line) }) }
	blockCap := stopHookBlockCap()
	for {
		turn, err := runOneTurnSig(ctx, cfg, inv, tr, bg)
		if err != nil {
			return err
		}
		if turn.lastText != "" {
			lastText = turn.lastText
		}
		if turn.done {
			final.Hold(turn.resultLine)
			if nested {
				finish()
				return nil
			}
			if resultFailed(turn.resultLine) {
				finish()
				return errRunFailed
			}
			// sr:provides stop-hook-payload/claude
			stop := corehooks.NewStop(lastText, stopBlocks)
			active, last := stop.Continuing, stop.LastMessage
			running := bg.running()
			crons := sessionCrons(cfg.wake)
			stopOut, stopRuns, stopErr := inv.FireRuns(ctx, hooks.Input{
				SessionID:            cfg.SessionID,
				Cwd:                  cfg.Cwd,
				HookEventName:        hooks.EventStop,
				StopHookActive:       &active,
				LastAssistantMessage: &last,
				BackgroundTasks:      &running,
				SessionCrons:         &crons,
			})
			writeStopHookError(cfg, stopRuns)
			// Its feedback, attachment and stop_hook_summary are written as it
			// fires (transcript.recordHookRuns).
			// sr:provides stop-block-continuation/claude
			if turnloop.Continues(stopErr != nil, stopOut.Decision == "block") {
				stopBlocks++
				// sr:provides stop-block-cap/claude
				if turnloop.AfterBlock(stopBlocks, blockCap) {
					// Re-prompt: the turn goes on, so the script runs again and
					// reacts to the block. Its result frame is dropped — a
					// continued turn ends with one result, at its real end
					// (claude 2.1.282 streamed a single result across 8
					// continuations).
					lastSig, repeats = "", 0
					final.Continue()
					continue
				}
				// The block cap: real Claude Code lets a Stop block the turn
				// CLAUDE_CODE_STOP_HOOK_BLOCK_CAP (default 8) times in a row;
				// the next block is overridden and the turn ends, with a
				// warning record (the 2.1.282 binary: `ve>xe`; a controlled run
				// fired Stop 9 times). 0 disables the cap.
				writeCapOverride(tr, stopBlocks)
				// The overridden turn's result carries no text: claude
				// 2.1.282 streamed "result":"" after the override.
				final.Hold(withEmptyResult(turn.resultLine))
			}
			stopBlocks = 0
			finish()
			// The turn is over. Hand over what finished in the background, one
			// new turn per task, waiting while a background agent still runs.
			// A notification UserPromptSubmit refuses starts no turn; the next
			// finished task is handed over instead (tasks.NextTurn).
			// The wait for background agents ends after the idle ceiling.
			waitCtx, cancelWait := tasks.WaitCeiling(ctx, cfg.BgWaitCeiling)
			next := bg.NextTurn(waitCtx, cfg.AgentID, func(t *tasks.Task) bool { return bg.deliverAsTurn(ctx, cfg, inv, tr, t) })
			cancelWait()
			if next != nil {
				lastSig, repeats = "", 0
				continue
			}
			bg.ReapAtExit(cfg.AgentID, printReapGrace)
			return nil
		}
		sig := turn.sig
		if sig != "" && sig == lastSig {
			repeats++
			// repeats counts the emissions after the first: the run ends on the
			// maxIdenticalTurns-th identical one in a row.
			if repeats+1 >= maxIdenticalTurns {
				return fmt.Errorf("claude-mock: scenario looped — the same tool_use was emitted %d times in a row without advancing (signature %q); the script is re-run once per turn and must vary its output based on conversation history — read $A10N_MOCK_SESSION_FILE (e.g. grep for a prior tool_result/tool_use_id) and emit the next step (or a final result) instead of re-emitting the same tool_use", maxIdenticalTurns, sig)
			}
		} else {
			repeats = 0
		}
		lastSig = sig
	}
}
