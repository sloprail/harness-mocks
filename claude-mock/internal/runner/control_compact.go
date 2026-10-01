package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/sloprail/harness-mocks/claude-mock/internal/hooks"
	"github.com/sloprail/harness-mocks/internal/compaction"
)

// compact is a compaction, the way real Claude Code performs one. Evidence
// (EVIDENCE.md): two manual /compact runs through claude 2.1.282 (fixtures
// evidence/compact*), the 65 compact_boundary records in the real transcripts,
// and the binary's compaction path.
//
//  1. PreCompact fires {trigger, custom_instructions: null}; an exit 2 or a
//     decision:block stops the compaction — nothing is written.
//  2. For a MANUAL compaction, SubagentStop fires for the summarizer that
//     wrote the summary (fireSummarizerStop). (Not measured for an automatic
//     compaction, so not fired for one.)
//  3. A compact_boundary is APPENDED to the file the session is writing (see
//     writeCompactBoundary), and the summary (isCompactSummary,
//     isVisibleInTranscriptOnly) chains to it.
//  4. SessionStart fires with source "compact", then PostCompact
//     {trigger, compact_summary}. Neither Pre- nor PostCompact leaves an
//     attachment; their outcome is display text.
//  5. A manual compaction is the /compact command, so the three records a
//     local command leaves follow the summary (writeCompactCommand), and
//     SessionStart:compact's attachments come after them.
//
// The order is the compaction capability's (internal/compaction); this supplies
// each step. It returns whether the compaction happened, or an error for a
// control record that asks for something impossible. The scenario drives it with
// {"type":"compact"[,"summary":…][,"trigger":"auto"|"manual"][,"preserve":N]
// [,"pre_tokens":N][,"post_tokens":N][,"preserved_segment":false][,"tail_offset":K]
// [,"logical_parent":…][,"id":…]}, or with its own
// isCompactSummary record, which is used as the summary.
//
// sr:provides manual-compaction/claude
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
	preserve := defaultPreserved
	if rec.Preserve != nil {
		preserve = *rec.Preserve
	}
	started := time.Now()
	var preRuns, postRuns []hooks.HandlerRun
	var sum map[string]any
	var anchor, summaryText string
	happened := compaction.Run(trigger == "manual", compaction.Steps{
		Before: func() bool {
			preOut, preErr := inv.WithRecorder(func(_ hooks.Input, runs []hooks.HandlerRun) { preRuns = runs }).Fire(ctx, hooks.Input{
				SessionID: cfg.SessionID, Cwd: cfg.Cwd, HookEventName: hooks.EventPreCompact,
				Trigger: trigger, CustomInstructions: json.RawMessage("null"),
			})
			if preErr != nil || preOut.Decision == "block" {
				// For a manual /compact the block's message is shown to the user.
				if msg := compactBlockMessage(preOut, preErr); trigger == "manual" && msg != "" {
					fmt.Fprintln(cfg.Stderr, msg)
				}
				fmt.Fprintf(cfg.Stderr, "claude-mock: PreCompact blocked the compaction\n")
				return true
			}
			var ok bool
			sum, anchor, summaryText, ok = compactSummary(rec, line)
			if ok {
				writeCompactingStatus(cfg)
			}
			return !ok
		},
		Summarizer: func() { fireSummarizerStop(ctx, cfg, inv, tr, summaryText) },
		Boundary: func() {
			writeCompactBoundary(cfg, tr, compactionSpec{
				logicalParent: rec.LogicalParent, preserve: preserve, anchor: anchor, trigger: trigger,
				preTokens: rec.PreTokens, postTokens: rec.PostTokens, durationMs: time.Since(started).Milliseconds(),
				withSegment: rec.PreservedSegment == nil || *rec.PreservedSegment, tailOffset: rec.TailOffset,
			})
		},
		Summary: func() {
			sumLine, _ := marshalRecord(sum)
			writeStreamLine(cfg, sumLine)
			tr.persist(sumLine)
		},
		Resume: func() {
			ssInv := inv
			if trigger == "manual" {
				ssInv = inv.WithRecorder(tr.holdHookRuns)
			}
			fireCompactedStart(ctx, cfg, ssInv)
		},
		After: func() {
			_, _ = inv.WithRecorder(func(_ hooks.Input, runs []hooks.HandlerRun) { postRuns = runs }).Fire(ctx, hooks.Input{
				SessionID: cfg.SessionID, Cwd: cfg.Cwd, HookEventName: hooks.EventPostCompact,
				Trigger: trigger, CompactSummary: &summaryText,
			})
		},
		Command: func() { writeCompactCommand(cfg, tr, preRuns, postRuns) },
	})
	return happened, nil
}
