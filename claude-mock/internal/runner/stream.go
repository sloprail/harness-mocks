package runner

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strings"

	"github.com/sloprail/harness-mocks/claude-mock/internal/hooks"
	"github.com/sloprail/harness-mocks/claude-mock/internal/toolexec"
)

// streamAndHook runs the scenario script in a turn-based loop, reads its JSONL
// output, fires hooks, executes tool calls, and writes valid lines to cfg.Out.
//
// Turn-based protocol:
//  1. Script runs and emits JSONL records.
//  2. Each forwarded record is appended to the session JSONL file so the script
//     can read the full conversation history on subsequent turns.
//  3. When the script emits an assistant record with a tool_use block, the mock
//     executes the tool locally, synthesises a user record with a tool_result
//     block, writes it to stdout AND the session file, then re-invokes the script.
//  4. The script receives A10N_MOCK_SESSION_FILE pointing at the session JSONL.
//     It can use any shell tool (grep/tail/jq) to inspect history and decide what
//     to emit next — no single-value env vars are needed.
//  5. Repeat until the script emits a result frame or exits.
//
// sr:docs https://code.claude.com/docs/en/env-vars (CLAUDE_CONFIG_DIR)
// sr:docs https://docs.anthropic.com/en/docs/claude-code/sdk#stream-json-output-format
// maxIdenticalTurns bounds how many times in a row the script may emit the SAME
// pending tool_use (same name+input). A static scenario that doesn't advance its
// output based on conversation history would otherwise loop forever (re-run →
// same tool_use → re-run …). Hitting the bound is a scenario bug, surfaced as an
// error instead of a hang.
const maxIdenticalTurns = 5

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
func streamAndHook(ctx context.Context, cfg Config, inv *hooks.Invoker, tr *transcript) error {
	// Every FILE record persisted below goes through tr, which chains it from the
	// one before it (real-CC shape — see sessionWriter). The chain seeds from the
	// last uuid already on disk, so a resume continues the existing chain. The
	// STDOUT stream is written directly with cfg.Out and is NOT chained (it must
	// stay the mock's claude stream).
	if cfg.bg == nil {
		cfg.bg = newBackgroundTasks()
		defer cfg.bg.shutdown()
	}
	bg := cfg.bg
	nested := cfg.SuppressSubagentHooks
	var lastSig string
	var repeats int
	var stopBlocks int
	var lastText string
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
			if nested {
				bg.stopOwned(cfg)
				return nil
			}
			active := stopBlocks > 0
			tasks := bg.running()
			crons := []any{}
			last := lastText
			stopOut, stopErr := inv.Fire(ctx, hooks.Input{
				SessionID:            cfg.SessionID,
				Cwd:                  cfg.Cwd,
				HookEventName:        hooks.EventStop,
				StopHookActive:       &active,
				LastAssistantMessage: &last,
				BackgroundTasks:      &tasks,
				SessionCrons:         &crons,
			})
			// Its feedback, attachment and stop_hook_summary are written as it
			// fires (transcript.recordHookRuns).
			if stopErr != nil || stopOut.Decision == "block" {
				stopBlocks++
				if blockCap > 0 && stopBlocks >= blockCap {
					// Stop kept blocking — give up (the real block-cap backstop).
					// blockCap == 0 means unlimited (same convention as agent.go's
					// SubagentStop loop).
					bg.stopOwned(cfg)
					return nil
				}
				// Re-prompt: continue the loop so the next script turn reacts to the block.
				lastSig, repeats = "", 0
				continue
			}
			stopBlocks = 0
			// The turn is over. Hand over what finished in the background, one
			// new turn per task, waiting while a background agent still runs.
			if t := bg.awaitAfterTurn(ctx, cfg.AgentID); t != nil {
				if bg.deliverAsTurn(ctx, cfg, inv, tr, t) {
					lastSig, repeats = "", 0
					continue
				}
				continue
			}
			bg.stopOwned(cfg)
			return nil
		}
		sig := turn.sig
		if sig != "" && sig == lastSig {
			repeats++
			if repeats >= maxIdenticalTurns {
				return fmt.Errorf("claude-mock: scenario looped — the same tool_use was emitted %d times in a row without advancing (signature %q); the script is re-run once per turn and must vary its output based on conversation history — read $A10N_MOCK_SESSION_FILE (e.g. grep for a prior tool_result/tool_use_id) and emit the next step (or a final result) instead of re-emitting the same tool_use", maxIdenticalTurns, sig)
			}
		} else {
			repeats = 0
		}
		lastSig = sig
	}
}

// turnResult is one script invocation's outcome.
type turnResult struct {
	// done: the script ended its turn (result frame, or it exited having
	// emitted neither a tool_use nor a compaction).
	done bool
	// sig identifies what the invocation did when it did not end the turn (the
	// pending tool_use, or a compaction), for the loop guard.
	sig string
	// lastText is the text of the last assistant record it emitted.
	lastText string
}

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
			return turnResult{done: true, lastText: sc.lastText}, nil
		}
		// The invocation compacted the context and stopped: the turn goes on
		// after a compaction, so the script runs again.
		return turnResult{sig: sc.compactSig, lastText: sc.lastText}, nil
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
	switch {
	case isAgentTool(pending.ToolName) && runsInBackground(pending.ToolInput):
		res, startAgent = bg.launchAgent(cfg, inv, pending.ToolUseID, pending.ToolInput, tr)
	case isAgentTool(pending.ToolName):
		res = runAgentTool(ctx, cfg, inv, pending.ToolUseID, pending.ToolInput, tr)
	case isScheduleWakeupTool(pending.ToolName):
		res = runScheduleWakeupTool(pending.ToolInput)
	case pending.ToolName == "Bash" && runsInBackground(pending.ToolInput):
		res = bg.launchBash(cfg, pending.ToolUseID, pending.ToolInput)
	default:
		// cfg.SessionID is the session the Bash tool exports as CLAUDE_CODE_SESSION_ID.
		// A subagent's nested run carries the PARENT's session id (subagentRun.run), the
		// same id its hooks get — real claude shares one session_id across subagents.
		res = toolexec.Execute(ctx, pending.ToolName, pending.ToolInput, cfg.Cwd, cfg.SessionID)
	}

	// Synthesise and emit the tool_result user record.
	// sr:docs https://docs.anthropic.com/en/docs/claude-code/sdk#stream-json-output-format
	if err := emitToolResult(cfg, pending, res, tr); err != nil {
		return turnResult{}, err
	}

	// PostToolUse for the synthesised result — not for a call the tool refused
	// (real Claude Code fires PostToolUseFailure for those, which the mock does
	// not model). tool_response is the tool's structured result where it has
	// one, else the text the model got.
	// sr:docs https://docs.anthropic.com/en/docs/claude-code/hooks#posttooluse
	if !res.IsError {
		_, _ = inv.Fire(ctx, hooks.Input{
			SessionID:     cfg.SessionID,
			Cwd:           cfg.Cwd,
			HookEventName: hooks.EventPostToolUse,
			ToolName:      pending.ToolName,
			ToolUseID:     pending.ToolUseID,
			ToolInput:     pending.ToolInput,
			ToolResponse:  toolResponse(res),
		})
	}
	if startAgent != nil {
		startAgent()
	}

	// A background task that finished while this tool ran is handed over now,
	// inside the turn.
	bg.deliverMidTurn(ctx, cfg, inv, tr)

	return turnResult{sig: pending.ToolName + ":" + string(pending.ToolInput), lastText: sc.lastText}, nil
}

// toolResponse is PostToolUse's tool_response for a result.
func toolResponse(res toolexec.Result) json.RawMessage {
	var v any = res.Output
	if res.ToolUseResult != nil {
		v = res.ToolUseResult
	}
	b, _ := marshalRecord(v)
	return b
}

// pendingToolUse carries the fields needed to execute a tool and synthesise the
// tool_result record.
type pendingToolUse struct {
	ToolUseID string
	ToolName  string
	ToolInput json.RawMessage

	// Blocked is set when a PreToolUse hook REFUSED this tool call — an exit-0
	// permissionDecision deny (or deprecated decision:block), or an exit 2. The
	// tool is NOT executed; BlockReason is what the refusal tool_result quotes.
	Blocked     bool
	BlockReason string
}

// scanResult is what one script invocation's output amounted to.
type scanResult struct {
	pending    pendingToolUse
	done       bool   // a result frame was seen
	compactSig string // a compaction happened (and what it was)
	lastText   string // text of the last assistant record
}

// scanLines reads one script invocation's JSONL output line by line, until a
// tool_use (returned as pending), a result frame (done), or the end of output.
func scanLines(ctx context.Context, r io.Reader, cfg Config, inv *hooks.Invoker, tr *transcript) (scanResult, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	var out scanResult

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}

		rec, err := validateRecord(line)
		if err != nil {
			fmt.Fprintf(cfg.Stderr, "claude-mock: invalid JSONL line: %v\nline: %s\n", err, line)
			return scanResult{}, fmt.Errorf("claude-mock: script emitted invalid JSONL: %w", err)
		}

		// A compaction: the {"type":"compact",…} control record, or a scenario's
		// own isCompactSummary record. See compact (control.go). The turn goes on
		// after it.
		// sr:docs https://code.claude.com/docs/en/hooks#precompact
		if rec.Type == "compact" || rec.IsCompactSummary {
			if compact(ctx, cfg, inv, tr, rec, line) {
				out.compactSig += "compact:" + string(line)
			}
			continue
		}

		// Control records: fire hooks, do NOT forward to stdout or session.
		if handled, err := handleControlRecord(ctx, rec, line, cfg, inv, tr); err != nil {
			return scanResult{}, err
		} else if handled {
			continue
		}

		if rec.Type == "assistant" {
			if t := assistantText(line); t != "" {
				out.lastText = t
			}
		}

		// PreToolUse + turn break on tool_use blocks.
		// sr:docs https://docs.anthropic.com/en/docs/claude-code/hooks#pretooluse
		if rec.Type == "assistant" {
			toolUseID, toolName, toolInput := extractFirstToolUseWithID(line)
			if toolName != "" {
				// Forward the assistant record + append to session BEFORE the hook
				// fires — real Claude Code writes the tool_use first and the
				// PreToolUse hook's attachment after it, and the tool_use is part
				// of the trajectory whatever the hook decides.
				writeStreamLine(cfg, line)
				tr.persist(line)

				hookOut, hookErr := inv.Fire(ctx, hooks.Input{
					SessionID:     cfg.SessionID,
					AgentID:       cfg.AgentID,
					Cwd:           cfg.Cwd,
					HookEventName: hooks.EventPreToolUse,
					ToolName:      toolName,
					ToolUseID:     toolUseID,
					ToolInput:     toolInput,
				})
				out.pending = pendingToolUse{ToolUseID: toolUseID, ToolName: toolName, ToolInput: toolInput}
				var blockErr *hooks.BlockError
				switch {
				case errors.As(hookErr, &blockErr):
					fmt.Fprintf(cfg.Stderr, "claude-mock: PreToolUse hook blocked: %v\n", hookErr)
					out.pending.Blocked, out.pending.BlockReason = true, blockErr.Quoted()
				case hookErr != nil:
					return scanResult{}, hookErr
				case isDeny(hookOut):
					out.pending.Blocked, out.pending.BlockReason = true, denyReason(hookOut)
				}
				return out, nil
			}
		}

		// Forward line to the STDOUT stream always — it is the mock's claude-compatible
		// output and every record (result included) belongs on it.
		writeStreamLine(cfg, line)

		// The FILE is different from the stream. Real Claude Code NEVER persists the
		// `result` frame to the transcript file — verified: 0 type:"result" records in a
		// real ~/.claude/projects/<proj>/<session>.jsonl, even though the frame is on the
		// --output-format stream-json STDOUT. The `result` is a stdout-stream-only frame;
		// the transcript ends at the last assistant/tool_result record. So stream it
		// (above) but do NOT persist it. Every other record is chained into the file.
		if rec.Type != "result" {
			tr.persist(line)
		}

		// PostToolUse for a tool_result the scenario wrote itself (a tool the
		// mock does not run, e.g. an AskUserQuestion answer). Real PostToolUse
		// names the call by its tool_use_id — also its attachment's toolUseID —
		// and the tool by the tool_use it answers.
		// sr:docs https://docs.anthropic.com/en/docs/claude-code/hooks#posttooluse
		if rec.Type == "user" {
			toolUseID, toolName, toolOutput := extractFirstToolResult(line)
			if toolName == "" && toolUseID != "" {
				toolName = toolNameInTranscript(tr, toolUseID)
			}
			if toolName != "" {
				_, _ = inv.Fire(ctx, hooks.Input{
					SessionID:     cfg.SessionID,
					Cwd:           cfg.Cwd,
					HookEventName: hooks.EventPostToolUse,
					ToolName:      toolName,
					ToolUseID:     toolUseID,
					ToolResponse:  toolOutput,
				})
			}
		}

		if rec.Type == "result" {
			out.done = true
			return out, nil
		}
	}

	if err := scanner.Err(); err != nil {
		slog.Debug("claude-mock: scanner error", "err", err)
		return scanResult{}, err
	}
	return out, nil
}

// buildEnv constructs the environment for a script invocation.
// A10N_MOCK_SESSION_FILE points at the session JSONL so the script can read the
// full conversation history with any shell tool.
// CLAUDE_CONFIG_DIR is set to the same dir so that tooling that reads Claude
// Code config also finds the mock's session files.
// sr:docs https://code.claude.com/docs/en/env-vars (CLAUDE_CONFIG_DIR)
func buildEnv(cfg Config, tr *transcript) []string {
	sessionPath := ""
	if tr != nil {
		// Opened (and created) here if nothing has been written yet: the script
		// reads history from it, and a script run always follows the prompt.
		if f := tr.file(); f != nil {
			sessionPath = f.Name()
		}
	}
	return append(os.Environ(),
		// CLAUDE_CODE_SESSION_ID mirrors the real claude CLI, which exports the active
		// session id into every Bash-tool subprocess. Tools that resolve "the current
		// session" (e.g. a10n-task-executor session autopilot) read it.
		// sr:docs https://code.claude.com/docs/en/env-vars (CLAUDE_CODE_SESSION_ID)
		"CLAUDE_CODE_SESSION_ID="+cfg.SessionID,
		"A10N_MOCK_SESSION_ID="+cfg.SessionID,
		"A10N_MOCK_PROMPT="+cfg.Prompt,
		"A10N_MOCK_ADDITIONAL_CONTEXT="+cfg.AdditionalContext,
		"A10N_MOCK_IS_RESUME="+boolStr(cfg.IsResume),
		"A10N_MOCK_SESSION_FILE="+sessionPath,
		"CLAUDE_CONFIG_DIR="+cfg.ConfigDir,
	)
}

// emitToolResult writes a synthetic user record with a tool_result block to the
// STDOUT stream and, chained into the transcript, to the session FILE.
//
// The record carries no uuid/parentUuid of its own — sw.persist mints a uuid and
// chains it from the previous persisted record, which is exactly the shape a real
// tool_result (type:"user") has on disk: uuid + a non-null parentUuid. The STDOUT
// copy stays uuid-less, matching the mock's claude stream (the stream frames carry no
// transcript uuid; the FILE is where the chained identity lives).
//
// The content is a string — what real Claude Code writes for a tool's text
// result, a refusal and an error alike — or, where the real tool answers with
// one (an async Agent receipt), a list of text blocks.
//
// sr:docs https://docs.anthropic.com/en/docs/claude-code/sdk#stream-json-output-format
func emitToolResult(cfg Config, call pendingToolUse, res toolexec.Result, tr *transcript) error {
	// A result with no text is given to the model as "(<Tool> completed with
	// no output)" — claude 2.1.282 replaces any empty or whitespace-only tool
	// result content with it (3,479 "(Bash completed with no output)" results
	// in the real transcripts, 0 empty ones).
	if strings.TrimSpace(res.Output) == "" {
		res.Output = "(" + call.ToolName + " completed with no output)"
	}
	var content any = res.Output
	if res.ContentAsBlocks {
		content = []map[string]any{{"type": "text", "text": res.Output}}
	}

	record := map[string]any{
		"type": "user",
		"message": map[string]any{
			"role": "user",
			"content": []map[string]any{
				{
					"type":        "tool_result",
					"tool_use_id": call.ToolUseID,
					"content":     content,
					"is_error":    res.IsError,
				},
			},
		},
	}

	line, err := marshalRecord(record)
	if err != nil {
		return fmt.Errorf("claude-mock: marshal tool_result: %w", err)
	}
	writeStreamLine(cfg, line)

	// The FILE copy carries what real Claude Code puts beside a tool_result:
	// toolUseResult, the tool's structured result (a background launch's
	// backgroundTaskId, an async agent's agentId), where the tool gives one.
	if res.ToolUseResult != nil {
		record["toolUseResult"] = res.ToolUseResult
		if withResult, err := marshalRecord(record); err == nil {
			line = withResult
		}
	}
	tr.persist(line)
	return nil
}

// toolNameInTranscript is the name of the tool_use with id toolUseID in the
// transcript, or "".
func toolNameInTranscript(tr *transcript, toolUseID string) string {
	data, err := os.ReadFile(tr.path)
	if err != nil {
		return ""
	}
	for _, line := range bytes.Split(data, []byte{'\n'}) {
		if !bytes.Contains(line, []byte(toolUseID)) {
			continue
		}
		if id, name, _ := extractFirstToolUseWithID(line); id == toolUseID {
			return name
		}
	}
	return ""
}
