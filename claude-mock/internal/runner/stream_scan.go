package runner

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"

	"github.com/sloprail/harness-mocks/claude-mock/internal/hooks"
)

// scanLines reads one script invocation's JSONL output line by line, until a tool_use (pending), a result frame (done) or its end.
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

		if rec.Type == "gate" { // the step that follows waits for what the gate names: the script orders the agents
			cfg.steps.hold(ctx, cfg, rec.Gate)
			out.execGate = rec.Gate
			continue
		}

		// A compaction: the {"type":"compact",…} control record, or a scenario's
		// own isCompactSummary record. See compact (control.go). The turn goes on
		// after it.
		// sr:docs https://code.claude.com/docs/en/hooks#precompact
		if rec.Type == "compact" || rec.IsCompactSummary {
			done, err := compact(ctx, cfg, inv, tr, rec, line)
			if err != nil {
				return scanResult{}, err
			}
			if done {
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
				if _, err := Schema().Check(toolName, toolInput); err != nil { // adr/tool-calls-validated
					return scanResult{}, err
				}
				line, streamed, toolInput := withToolDefaults(cfg, line)
				writeToolUse(cfg, streamed, toolName, toolInput)
				tr.persist(line)
				if res := invalidCall(toolName, toolInput, cfg.Cwd); res != nil {
					out.pending = pendingToolUse{ToolUseID: toolUseID, ToolName: toolName, ToolInput: toolInput, Invalid: res}
					cfg.steps.started()
					return out, nil
				}
				pre := hooks.Input{
					SessionID:     cfg.SessionID,
					AgentID:       cfg.AgentID,
					Cwd:           cfg.Cwd,
					HookEventName: hooks.EventPreToolUse,
					ToolName:      toolName,
					ToolUseID:     toolUseID,
					ToolInput:     toolInput,
				}
				hookOut, preRuns, hookErr := inv.FireRuns(ctx, pre)
				writeHookEventFrames(cfg, pre, preRuns)
				out.pending = pendingToolUse{ToolUseID: toolUseID, ToolName: toolName, ToolInput: toolInput}
				if err := decidePreTool(cfg, &out.pending, hookOut, hookErr); err != nil {
					return scanResult{}, err
				}
				cfg.steps.started() // once its PreToolUse hooks have run: what another agent's gate may wait for
				return out, nil
			}
		}

		// Forward line to the STDOUT stream — it is the mock's claude-compatible
		// output. The result frame is held back: it ends the turn only if Stop
		// lets the turn end (see streamAndHook).
		if rec.Type == "result" {
			out.resultLine = append([]byte(nil), line...)
			cfg.steps.answered()
		} else {
			writeStreamLine(cfg, line)
		}

		// The FILE is different from the stream. Real Claude Code NEVER persists the
		// `result` frame to the transcript file — verified: 0 type:"result" records in a
		// real ~/.claude/projects/<proj>/<session>.jsonl, even though the frame is on the
		// --output-format stream-json STDOUT. The `result` is a stdout-stream-only frame;
		// the transcript ends at the last assistant/tool_result record. So stream it
		// (above) but do NOT persist it. Every other record is chained into the file.
		if rec.Type != "result" {
			tr.persist(line)
		}

		if rec.Type == "user" {
			if err := postScenarioResult(ctx, cfg, inv, tr, line); err != nil {
				return scanResult{}, err
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
