package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/sloprail/harness-mocks/claude-mock/internal/hooks"
)

// compact is a compaction, the way real Claude Code performs one. Evidence
// (EVIDENCE.md): two manual /compact runs through claude 2.1.282 (fixtures
// evidence/compact*), the 65 compact_boundary records in the real transcripts,
// and the binary's compaction path.
//
//  1. PreCompact fires {trigger, custom_instructions: null}; an exit 2 or a
//     decision:block stops the compaction — nothing is written.
//  2. For a MANUAL compaction, SubagentStop fires for the summarizer that
//     wrote the summary: agent_type "", a fresh agent_id, an
//     agent_transcript_path no file is ever written to, and the summary as
//     last_assistant_message. (Not measured for an automatic compaction, so
//     not fired for one.)
//  3. A compact_boundary is APPENDED to the file the session is writing (see
//     writeCompactBoundary), and the summary (isCompactSummary,
//     isVisibleInTranscriptOnly) chains to it.
//  4. SessionStart fires with source "compact", then PostCompact
//     {trigger, compact_summary}. Neither Pre- nor PostCompact leaves an
//     attachment; their outcome is display text.
//  5. A manual compaction is the /compact command, so the three records a
//     local command leaves follow the summary — the caveat, the command, and
//     its output "Compacted <what the Pre/PostCompact hooks reported>" — and
//     SessionStart:compact's attachments come after them.
//
// It returns whether the compaction happened, or an error for a control
// record that asks for something impossible. The scenario drives it with
// {"type":"compact"[,"summary":…][,"trigger":"auto"|"manual"][,"preserve":N]
// [,"pre_tokens":N][,"post_tokens":N][,"preserved_segment":false][,"tail_offset":K]
// [,"logical_parent":…][,"id":…]}, or with its own
// isCompactSummary record, which is used as the summary.
// sr:docs https://code.claude.com/docs/en/hooks#precompact
// sr:docs https://code.claude.com/docs/en/hooks#postcompact
func compact(ctx context.Context, cfg Config, inv *hooks.Invoker, tr *transcript, rec *cliRecord, line []byte) (bool, error) {
	// A tail_offset has to leave at least one written record for the segment
	// to end on; a scenario asking for more is a scenario bug, not something
	// to fall back from silently.
	if rec.TailOffset < 0 {
		return false, fmt.Errorf("claude-mock: compact: tail_offset must not be negative, got %d", rec.TailOffset)
	}
	if rec.TailOffset > 0 {
		if rec.PreservedSegment != nil && !*rec.PreservedSegment {
			return false, fmt.Errorf("claude-mock: compact: tail_offset needs a preserved segment, but preserved_segment is false")
		}
		if have := len(tr.lastUUIDs(rec.TailOffset + 1)); have <= rec.TailOffset {
			return false, fmt.Errorf("claude-mock: compact: tail_offset %d leaves no record for the preserved segment to end on: only %d records are written", rec.TailOffset, have)
		}
	}
	trigger := rec.Trigger
	if trigger == "" {
		trigger = "auto"
	}
	manual := trigger == "manual"
	started := time.Now()
	var preRuns, postRuns []hooks.HandlerRun
	preOut, preErr := inv.WithRecorder(func(_ hooks.Input, runs []hooks.HandlerRun) { preRuns = runs }).Fire(ctx, hooks.Input{
		SessionID: cfg.SessionID, Cwd: cfg.Cwd, HookEventName: hooks.EventPreCompact,
		Trigger: trigger, CustomInstructions: json.RawMessage("null"),
	})
	if preErr != nil || preOut.Decision == "block" {
		fmt.Fprintf(cfg.Stderr, "claude-mock: PreCompact blocked the compaction\n")
		return false, nil
	}

	// The summary: the scenario's own record, or one built from the control
	// record. Its uuid is minted up front — the boundary names it as anchor.
	var sum map[string]any
	if rec.IsCompactSummary {
		if json.Unmarshal(line, &sum) != nil {
			return false, nil
		}
	} else {
		text := rec.Summary
		if text == "" {
			text = "This session is being continued from a previous conversation that ran out of context."
		}
		sum = map[string]any{
			"type": "user", "isCompactSummary": true,
			"message": map[string]any{"role": "user", "content": text},
		}
		if rec.ID != "" {
			sum["id"] = rec.ID
		}
	}
	if _, ok := sum["isVisibleInTranscriptOnly"]; !ok {
		sum["isVisibleInTranscriptOnly"] = true // every one of the 66 real summaries
	}
	anchor, _ := sum["uuid"].(string)
	if anchor == "" {
		anchor = newRecordUUID()
		sum["uuid"] = anchor
	}
	delete(sum, "parentUuid") // chains to the boundary
	summaryText := ""
	if m, ok := sum["message"].(map[string]any); ok {
		summaryText, _ = m["content"].(string)
	}

	if manual {
		summarizer, err := newAgentID()
		if err == nil {
			active := false
			tasks := cfg.bg.running()
			crons := []any{}
			sessionFile := cfg.sessionFile
			if sessionFile == "" {
				sessionFile = tr.path
			}
			_, _ = inv.WithRecorder(nil).Fire(ctx, hooks.Input{
				SessionID: cfg.SessionID, Cwd: cfg.Cwd, HookEventName: hooks.EventSubagentStop,
				AgentID: summarizer, AgentType: "", StopHookActive: &active,
				AgentTranscriptPath:  filepath.Join(strings.TrimSuffix(sessionFile, ".jsonl"), "subagents", "agent-"+summarizer+".jsonl"),
				LastAssistantMessage: &summaryText, BackgroundTasks: &tasks, SessionCrons: &crons,
			})
		}
	}

	preserve := defaultPreserved
	if rec.Preserve != nil {
		preserve = *rec.Preserve
	}
	writeCompactBoundary(tr, compactionSpec{
		logicalParent: rec.LogicalParent, preserve: preserve, anchor: anchor, trigger: trigger,
		preTokens: rec.PreTokens, postTokens: rec.PostTokens, durationMs: time.Since(started).Milliseconds(),
		withSegment: rec.PreservedSegment == nil || *rec.PreservedSegment, tailOffset: rec.TailOffset,
	})
	sumLine, _ := marshalRecord(sum)
	writeStreamLine(cfg, sumLine)
	tr.persist(sumLine)

	ssInv := inv
	if manual {
		ssInv = inv.WithRecorder(tr.holdHookRuns)
	}
	fireSessionStart(ctx, cfg, ssInv, "compact")

	_, _ = inv.WithRecorder(func(_ hooks.Input, runs []hooks.HandlerRun) { postRuns = runs }).Fire(ctx, hooks.Input{
		SessionID: cfg.SessionID, Cwd: cfg.Cwd, HookEventName: hooks.EventPostCompact,
		Trigger: trigger, CompactSummary: &summaryText,
	})

	if manual {
		lines := append(compactHookLines("PreCompact", preRuns), compactHookLines("PostCompact", postRuns)...)
		tr.persistMap(map[string]any{"type": "user", "isMeta": true, "message": map[string]any{"role": "user",
			"content": "<local-command-caveat>Caveat: The messages below were generated by the user while running local commands. DO NOT respond to these messages or otherwise consider them in your response unless the user explicitly asks you to.</local-command-caveat>"}})
		tr.persistMap(map[string]any{"type": "user", "message": map[string]any{"role": "user",
			"content": "<command-name>/compact</command-name>\n            <command-message>compact</command-message>\n            <command-args></command-args>"}})
		tr.persistMap(map[string]any{"type": "user", "message": map[string]any{"role": "user",
			"content": "<local-command-stdout>Compacted " + strings.Join(lines, "\n") + "</local-command-stdout>"}})
		tr.flushHookRuns()
	}
	return true, nil
}
