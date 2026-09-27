package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/sloprail/harness-mocks/claude-mock/internal/hooks"
)

// handleControlRecord checks whether rec is a mock-only control record.
// If it is, the appropriate hook is fired and (true, err) is returned so the
// caller can skip forwarding the line to stdout.
// If rec is a normal stream-json record, (false, nil) is returned.
//
// Control record types:
//   - worktree_create / worktree_remove — fire WorktreeCreate / WorktreeRemove
//   - subagent_start                    — fire SubagentStart with explicit agent_type
//
// sr:docs https://docs.anthropic.com/en/docs/claude-code/hooks#worktreecreate
// sr:docs https://docs.anthropic.com/en/docs/claude-code/hooks#worktreeremove
// sr:docs https://docs.anthropic.com/en/docs/claude-code/hooks#subagentstart
func handleControlRecord(ctx context.Context, rec *cliRecord, line []byte, cfg Config, inv *hooks.Invoker, tr *transcript) (handled bool, err error) {
	switch rec.Type {
	case "worktree_create", "worktree_remove":
		evt := hooks.EventWorktreeCreate
		if rec.Type == "worktree_remove" {
			evt = hooks.EventWorktreeRemove
		}
		if _, err := inv.Fire(ctx, hooks.Input{
			SessionID:     cfg.SessionID,
			Cwd:           cfg.Cwd,
			HookEventName: evt,
			WorktreeName:  rec.WorktreeName,
		}); err != nil {
			fmt.Fprintf(cfg.Stderr, "claude-mock: %s hook blocked: %v\n", evt, err)
			return true, err
		}
		return true, nil

	case "subagent_start":
		if _, err := inv.Fire(ctx, hooks.Input{
			SessionID:     cfg.SessionID,
			Cwd:           cfg.Cwd,
			HookEventName: hooks.EventSubagentStart,
			AgentType:     rec.AgentType,
		}); err != nil {
			fmt.Fprintf(cfg.Stderr, "claude-mock: SubagentStart hook blocked: %v\n", err)
			return true, err
		}
		return true, nil
	}

	return false, nil
}

// defaultPreserved is how many of the last records a compaction keeps when
// the scenario does not say. Real compactions kept between 2 and 21 (every
// compact_boundary in the real transcripts); a manual /compact in a controlled
// claude 2.1.282 run kept 2.
const defaultPreserved = 2

// compact is a compaction, the way real Claude Code performs one (a manual
// /compact run through claude 2.1.282, the 65 compact_boundary records in the
// real transcripts, and the binary's compaction path):
//
//  1. PreCompact fires {trigger, custom_instructions}; an exit 2 or a
//     decision:block stops the compaction — nothing is written.
//  2. A compact_boundary is APPENDED to the file the session is writing:
//     parentless, its logicalParentUuid the last record before it (or the
//     scenario's logical_parent), its compactMetadata naming the records the
//     compaction kept — preservedMessages {anchorUuid, uuids, allUuids} and
//     preservedSegment {headUuid, anchorUuid, tailUuid}, the anchor being the
//     summary — plus trigger and token counts. The kept records stay where
//     they are; a fork copies them in after the summary (forkTranscript).
//  3. The summary (isCompactSummary) chains to the boundary.
//  4. SessionStart fires with source "compact"; its attachments follow the
//     summary.
//  5. PostCompact fires {trigger, compact_summary}. Neither Pre- nor
//     PostCompact leaves an attachment: their output is display text only.
//
// It returns whether the compaction happened. The scenario drives it with
// {"type":"compact"[,"summary":…][,"trigger":"auto"|"manual"][,"preserve":N]
// [,"pre_tokens":N][,"logical_parent":…][,"id":…]}, or with its own
// isCompactSummary record, which is used as the summary.
// sr:docs https://code.claude.com/docs/en/hooks#precompact
// sr:docs https://code.claude.com/docs/en/hooks#postcompact
func compact(ctx context.Context, cfg Config, inv *hooks.Invoker, tr *transcript, rec *cliRecord, line []byte) bool {
	trigger := rec.Trigger
	if trigger == "" {
		trigger = "auto"
	}
	started := time.Now()
	preOut, preErr := inv.Fire(ctx, hooks.Input{
		SessionID: cfg.SessionID, Cwd: cfg.Cwd, HookEventName: hooks.EventPreCompact,
		Trigger: trigger, CustomInstructions: json.RawMessage("null"),
	})
	if preErr != nil || preOut.Decision == "block" {
		fmt.Fprintf(cfg.Stderr, "claude-mock: PreCompact blocked the compaction\n")
		return false
	}

	// The summary: the scenario's own record, or one built from the control
	// record. Its uuid is minted up front — the boundary names it as anchor.
	var sum map[string]any
	if rec.IsCompactSummary {
		if json.Unmarshal(line, &sum) != nil {
			return false
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
	anchor, _ := sum["uuid"].(string)
	if anchor == "" {
		anchor = newRecordUUID()
		sum["uuid"] = anchor
	}
	delete(sum, "parentUuid") // chains to the boundary

	preserve := defaultPreserved
	if rec.Preserve != nil {
		preserve = *rec.Preserve
	}
	writeCompactBoundary(tr, compactionSpec{
		logicalParent: rec.LogicalParent, preserve: preserve, anchor: anchor, trigger: trigger,
		preTokens: rec.PreTokens, durationMs: time.Since(started).Milliseconds(),
	})
	sumLine, _ := marshalRecord(sum)
	writeStreamLine(cfg, sumLine)
	tr.persist(sumLine)

	fireSessionStart(ctx, cfg, inv, "compact")

	summaryText := ""
	if m, ok := sum["message"].(map[string]any); ok {
		summaryText, _ = m["content"].(string)
	}
	_, _ = inv.Fire(ctx, hooks.Input{
		SessionID: cfg.SessionID, Cwd: cfg.Cwd, HookEventName: hooks.EventPostCompact,
		Trigger: trigger, CompactSummary: &summaryText,
	})
	return true
}

// compactionSpec is what one compaction's boundary records.
type compactionSpec struct {
	logicalParent string
	preserve      int
	anchor        string
	trigger       string
	preTokens     int
	durationMs    int64
}

// writeCompactBoundary appends the compact_boundary a compaction opens with.
// It is APPENDED to the file the session is writing, not a new file: the file
// a real compaction happened in holds its boundary part-way down (46 of the 65
// real boundaries), and only a later resume moves the conversation to a new
// file.
func writeCompactBoundary(tr *transcript, spec compactionSpec) {
	kept := tr.lastUUIDs(spec.preserve)
	logicalParent := spec.logicalParent
	if logicalParent == "" {
		logicalParent = tr.lastUUID()
	}
	meta := map[string]any{
		"trigger":    spec.trigger,
		"preTokens":  spec.preTokens,
		"durationMs": spec.durationMs,
	}
	if len(kept) > 0 {
		meta["preservedSegment"] = map[string]any{
			"headUuid": kept[0], "anchorUuid": spec.anchor, "tailUuid": kept[len(kept)-1],
		}
		meta["preservedMessages"] = map[string]any{
			"anchorUuid": spec.anchor, "uuids": kept, "allUuids": kept,
		}
	}
	meta["postTokens"] = 0
	tr.persistMap(map[string]any{
		"parentUuid":        nil,
		"logicalParentUuid": logicalParent,
		"type":              "system",
		"subtype":           "compact_boundary",
		"content":           "Conversation compacted",
		"isMeta":            false,
		"level":             "info",
		"compactMetadata":   meta,
	})
}
