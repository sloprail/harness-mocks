package runner

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"

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

func streamAndHook(ctx context.Context, cfg Config, inv *hooks.Invoker, tr *transcript) error {
	// Every FILE record persisted below goes through tr, which chains it from the
	// one before it (real-CC shape — see sessionWriter). The chain seeds from the
	// last uuid already on disk, so a resume continues the existing chain. The
	// STDOUT stream is written directly with cfg.Out and is NOT chained (it must
	// stay the mock's claude stream).
	bg := newBackgroundTasks(cfg, tr)
	defer bg.wait()
	var lastSig string
	var repeats int
	var stopBlocks int
	blockCap := stopHookBlockCap()
	for {
		done, sig, err := runOneTurnSig(ctx, cfg, inv, tr, bg)
		if err != nil {
			return err
		}
		if done {
			// A SUB-AGENT run (SuppressSubagentHooks) must NOT fire Stop here — the Agent-tool
			// layer (agent.go) owns the sub-agent's terminal hook: it fires SubagentStop and runs
			// the block→re-run loop. Firing the ROOT Stop here would (a) wrongly run the root drain
			// inside a sub-agent and (b) consume the block-retry that agent.go expects to drive.
			if cfg.SuppressSubagentHooks {
				return nil
			}
			// ROOT agent: the script ended its turn (result frame). Real Claude Code fires the Stop
			// hook HERE; if the hook BLOCKS (exit 2 / exit-0 decision:block — e.g. an a10n drain's
			// "spawn one sub-agent per parked check" block), the turn is RE-PROMPTED and the agent
			// CONTINUES. We mirror that: fire Stop, surface its output into the transcript as an
			// attachment (so the next turn can read the reason/links), and if it blocked, LOOP again
			// instead of returning — bounded by the Stop-hook block cap. runner.go fires Stop only
			// on the error path (this owns the success path).
			// A background task that finished while the agent worked is delivered
			// before the turn is judged over: real Claude Code wakes the agent
			// with a <task-notification> turn, so the agent is not done yet.
			if bg.deliverFinished(true) {
				lastSig = ""
				repeats = 0
				continue
			}
			stopOut, stopErr := inv.Fire(ctx, hooks.Input{
				SessionID:      cfg.SessionID,
				Cwd:            cfg.Cwd,
				HookEventName:  hooks.EventStop,
				StopReason:     "end_turn",
				StopHookActive: stopBlocks > 0,
			})
			blocked := stopErr != nil || stopOut.Decision == "block"
			if blocked {
				// Its feedback and attachment are written as it fires
				// (transcript.recordHookRuns).
				stopBlocks++
				if blockCap > 0 && stopBlocks >= blockCap {
					// Stop kept blocking — give up (matches the real block-cap backstop).
					// blockCap == 0 means unlimited (same convention as agent.go's SubagentStop loop).
					return nil
				}
				// Re-prompt: continue the loop so the next script turn reacts to the block.
				lastSig = ""
				repeats = 0
				continue
			}
			return nil
		}
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

// runOneTurnSig executes the script once, processes its JSONL output, and returns:
//   - (true,  "",  nil)  when a result frame is seen (conversation complete)
//   - (false, sig, nil)  when a tool_use was executed; script should be re-run.
//     sig is the pending tool_use signature (name+input) for the loop guard.
//   - (false, "",  err)  on any error
func runOneTurnSig(ctx context.Context, cfg Config, inv *hooks.Invoker, tr *transcript, bg *backgroundTasks) (done bool, sig string, err error) {
	// Deliver what finished in the background since the last turn, so this turn's
	// script reads the notification in the transcript — the way real Claude Code
	// hands a completed task back to the agent.
	bg.deliverFinished(false)

	cmd := exec.CommandContext(ctx, "/bin/sh", cfg.ScriptPath) //nolint:gosec
	cmd.Dir = cfg.Cwd
	cmd.Env = buildEnv(cfg, tr)
	cmd.Stderr = cfg.Stderr

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return false, "", fmt.Errorf("claude-mock: stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return false, "", fmt.Errorf("claude-mock: start script: %w", err)
	}

	pending, done, scanErr := scanLines(ctx, stdoutPipe, cfg, inv, tr)
	waitErr := cmd.Wait()

	if scanErr != nil {
		return false, "", scanErr
	}
	if waitErr != nil {
		return false, "", waitErr
	}
	if done {
		return true, "", nil
	}

	// PreToolUse DENIED this tool call (exit-0 deny). Do NOT execute it; feed the
	// reason back as an (error) tool_result so the agent's next turn sees the block
	// and can self-correct + retry — the real Claude Code contract. No PostToolUse
	// fires (the tool never ran). The loop guard signature is the blocked tool_use so
	// an agent that re-emits the identical blocked call without adapting is still
	// bounded by maxIdenticalTurns.
	// sr:docs https://code.claude.com/docs/en/hooks#pretooluse
	if pending.Blocked {
		blockRes := toolexec.Result{
			Output:  "Tool call blocked by a PreToolUse hook: " + pending.BlockReason,
			IsError: true,
		}
		if err := emitToolResult(cfg, pending, blockRes, tr); err != nil {
			return false, "", err
		}
		return false, "blocked:" + pending.ToolName + ":" + string(pending.ToolInput), nil
	}

	// tool_use was seen — execute it.
	//
	// Two tools are special-cased here at the stream layer:
	//   - Agent (alias Task; real Claude renamed Task→Agent in v2.1.63) spawns a
	//     NESTED subagent run rather than a pure FS/Bash op, so it needs ctx, cfg,
	//     inv and the session file (none of which toolexec.Execute has access to).
	//   - ScheduleWakeup is routed to runScheduleWakeupTool purely for arg
	//     validation; on success it just returns a success tool_result. There is no
	//     real delay in the mock — the turn loop ALREADY re-runs the script after
	//     every tool_use, which IS the "wake-up fired, resume" behaviour. It has no
	//     compaction side effect (compaction is a separate event the SCRIPT emits as
	//     an isCompactSummary record; see scanLines).
	// sr:docs https://code.claude.com/docs/en/sub-agents
	var res toolexec.Result
	switch {
	case isAgentTool(pending.ToolName) && runsInBackground(pending.ToolInput):
		res = bg.launchAgent(ctx, inv, pending.ToolUseID, pending.ToolInput)
	case isAgentTool(pending.ToolName):
		res = runAgentTool(ctx, cfg, inv, pending.ToolUseID, pending.ToolInput, tr)
	case isScheduleWakeupTool(pending.ToolName):
		res = runScheduleWakeupTool(pending.ToolInput)
	case pending.ToolName == "Bash" && runsInBackground(pending.ToolInput):
		res = bg.launchBash(ctx, pending.ToolUseID, pending.ToolInput)
	case pending.ToolName == "TaskOutput":
		res = bg.output(ctx, pending.ToolInput)
	default:
		// cfg.SessionID is the session the Bash tool exports as CLAUDE_CODE_SESSION_ID.
		// A subagent's nested run carries the PARENT's session id (runSubagent), the
		// same id its hooks get — real claude shares one session_id across subagents.
		res = toolexec.Execute(ctx, pending.ToolName, pending.ToolInput, cfg.Cwd, cfg.SessionID)
	}

	// Synthesise and emit the tool_result user record.
	// sr:docs https://docs.anthropic.com/en/docs/claude-code/sdk#stream-json-output-format
	if err := emitToolResult(cfg, pending, res, tr); err != nil {
		return false, "", err
	}

	// PostToolUse for the synthesised result.
	// sr:docs https://docs.anthropic.com/en/docs/claude-code/hooks#posttooluse
	rawResult, _ := marshalRecord(res.Output)
	_, _ = inv.Fire(ctx, hooks.Input{
		SessionID:     cfg.SessionID,
		Cwd:           cfg.Cwd,
		HookEventName: hooks.EventPostToolUse,
		ToolName:      pending.ToolName,
		ToolUseID:     pending.ToolUseID,
		ToolInput:     pending.ToolInput,
		ToolOutput:    rawResult,
	})

	return false, pending.ToolName + ":" + string(pending.ToolInput), nil
}

// pendingToolUse carries the fields needed to execute a tool and synthesise the
// tool_result record.
type pendingToolUse struct {
	ToolUseID string
	ToolName  string
	ToolInput json.RawMessage

	// Blocked is set when a PreToolUse hook DENIED this tool call with
	// permissionDecision=deny (and EXIT 0 — the real Claude Code contract). The tool
	// is NOT executed; instead a tool_result carrying BlockReason is fed back to the
	// agent (the script's next turn), which may then self-correct and retry — exactly
	// as real Claude surfaces the deny reason and continues the turn rather than
	// aborting. (An exit-2 hook is a different, fatal path handled via Fire's error.)
	Blocked     bool
	BlockReason string
}

// scanLines reads one script invocation's JSONL output line by line.
// Returns (pending, done, err):
//   - done=true when a result frame is seen
//   - pending set when a tool_use was encountered (caller should execute + re-run)
func scanLines(ctx context.Context, r io.Reader, cfg Config, inv *hooks.Invoker, tr *transcript) (pending pendingToolUse, done bool, err error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}

		rec, err := validateRecord(line)
		if err != nil {
			fmt.Fprintf(cfg.Stderr, "claude-mock: invalid JSONL line: %v\nline: %s\n", err, line)
			return pendingToolUse{}, false, fmt.Errorf("claude-mock: script emitted invalid JSONL: %w", err)
		}

		// Control records: fire hooks, do NOT forward to stdout or session.
		if handled, err := handleControlRecord(ctx, rec, line, cfg, inv, tr); err != nil {
			return pendingToolUse{}, false, err
		} else if handled {
			continue
		}

		// Compaction record: real Claude Code writes a
		// {"type":"user","isCompactSummary":true,…} line when it auto-compacts the
		// context window. When the scenario emits one, forward it (it is part of the
		// trajectory/history) and react by firing SessionStart source="compact" — the
		// hook supplies the re-injected additionalContext; the record carries none. A
		// compaction record is NOT a tool_use, so it does not break the turn loop or
		// count toward the loop guard.
		// sr:docs https://code.claude.com/docs/en/hooks#sessionstart
		if rec.IsCompactSummary {
			cfg.Out.Write(line)         //nolint:errcheck
			cfg.Out.Write([]byte{'\n'}) //nolint:errcheck
			// Real Claude Code opens a compaction with a compact_boundary record —
			// parentless, naming the last record before it as its logical parent —
			// and chains the summary to it. See writeCompactBoundary.
			writeCompactBoundary(tr, "", 1)
			tr.persist(line)
			if _, err := fireSessionStart(ctx, cfg, inv, "compact"); err != nil {
				return pendingToolUse{}, false, fmt.Errorf("claude-mock: SessionStart (compact) hook blocked: %w", err)
			}
			continue
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
				cfg.Out.Write(line)         //nolint:errcheck
				cfg.Out.Write([]byte{'\n'}) //nolint:errcheck
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
				if hookErr != nil {
					fmt.Fprintf(cfg.Stderr, "claude-mock: PreToolUse hook blocked: %v\n", hookErr)
					return pendingToolUse{}, false, hookErr
				}
				denied := hookOut.Decision == "block" ||
					(hookOut.HookSpecificOutput != nil && hookOut.HookSpecificOutput.PermissionDecision == "deny")

				if denied {
					// Real Claude Code contract (empirically verified, STEP 0): a
					// PreToolUse deny with EXIT 0 BLOCKS this tool call and feeds the
					// reason back to the agent, which can then self-correct and retry —
					// the turn CONTINUES, it is not aborted. Model that by returning a
					// Blocked pending: the caller emits a tool_result carrying the reason
					// and re-runs the script (the agent's next turn) instead of erroring.
					reason := hookOut.Reason
					if hookOut.HookSpecificOutput != nil && hookOut.HookSpecificOutput.PermissionDecisionReason != "" {
						reason = hookOut.HookSpecificOutput.PermissionDecisionReason
					}
					return pendingToolUse{
						ToolUseID:   toolUseID,
						ToolName:    toolName,
						ToolInput:   toolInput,
						Blocked:     true,
						BlockReason: reason,
					}, false, nil
				}

				// Allowed → return for tool execution; the script is re-run with the
				// updated session file (now carrying this tool_use + its result).
				return pendingToolUse{
					ToolUseID: toolUseID,
					ToolName:  toolName,
					ToolInput: toolInput,
				}, false, nil
			}
		}

		// Forward line to the STDOUT stream always — it is the mock's claude-compatible
		// output and every record (result included) belongs on it.
		cfg.Out.Write(line)         //nolint:errcheck
		cfg.Out.Write([]byte{'\n'}) //nolint:errcheck

		// The FILE is different from the stream. Real Claude Code NEVER persists the
		// `result` frame to the transcript file — verified: 0 type:"result" records in a
		// real ~/.claude/projects/<proj>/<session>.jsonl, even though the frame is on the
		// --output-format stream-json STDOUT. The `result` is a stdout-stream-only frame;
		// the transcript ends at the last assistant/tool_result record. So stream it
		// (above) but do NOT persist it. Every other record is chained into the file.
		if rec.Type != "result" {
			tr.persist(line)
		}

		// PostToolUse for inline tool_result blocks (static scripts).
		// sr:docs https://docs.anthropic.com/en/docs/claude-code/hooks#posttooluse
		if rec.Type == "user" {
			toolName, toolOutput := extractFirstToolResult(line)
			if toolName != "" {
				_, _ = inv.Fire(ctx, hooks.Input{
					SessionID:     cfg.SessionID,
					Cwd:           cfg.Cwd,
					HookEventName: hooks.EventPostToolUse,
					ToolName:      toolName,
					ToolOutput:    toolOutput,
				})
			}
		}

		// SubagentStop on end_turn.
		// Suppressed for nested Agent-tool runs — see Config.SuppressSubagentHooks.
		// sr:docs https://docs.anthropic.com/en/docs/claude-code/hooks#subagentstop
		if rec.Type == "assistant" && rec.Message != nil && rec.Message.StopReason == "end_turn" {
			if cfg.IsResume && !cfg.SuppressSubagentHooks {
				_, _ = inv.Fire(ctx, hooks.Input{
					SessionID:     cfg.SessionID,
					Cwd:           cfg.Cwd,
					HookEventName: hooks.EventSubagentStop,
					StopReason:    "end_turn",
					AgentType:     "general-purpose",
				})
			}
		}

		if rec.Type == "result" {
			return pendingToolUse{}, true, nil
		}
	}

	if err := scanner.Err(); err != nil {
		slog.Debug("claude-mock: scanner error", "err", err)
		return pendingToolUse{}, false, err
	}
	return pendingToolUse{}, true, nil
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
// sr:docs https://docs.anthropic.com/en/docs/claude-code/sdk#stream-json-output-format
func emitToolResult(cfg Config, call pendingToolUse, res toolexec.Result, tr *transcript) error {
	content := res.Output
	if res.IsError {
		blocks, _ := marshalRecord([]map[string]string{
			{"type": "text", "text": res.Output},
		})
		content = string(blocks)
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
	cfg.Out.Write(line)         //nolint:errcheck
	cfg.Out.Write([]byte{'\n'}) //nolint:errcheck

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
