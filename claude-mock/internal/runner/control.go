package runner

import (
	"context"
	"fmt"

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
	case "compact":
		// The harness compacting the context, as real Claude Code writes it: a
		// compact_boundary record, then the summary chained to it, then
		// SessionStart with source="compact". The summary is forwarded on the
		// stream like any user record.
		// sr:docs https://code.claude.com/docs/en/hooks#sessionstart
		writeCompactBoundary(tr, rec.LogicalParent, 1)
		summary := rec.Summary
		if summary == "" {
			summary = "This session is being continued from a previous conversation that ran out of context."
		}
		sumLine, _ := marshalRecord(map[string]any{
			"type": "user", "isCompactSummary": true,
			"message": map[string]any{"role": "user", "content": summary},
		})
		cfg.Out.Write(sumLine)      //nolint:errcheck
		cfg.Out.Write([]byte{'\n'}) //nolint:errcheck
		tr.persist(sumLine)
		if _, err := fireSessionStart(ctx, cfg, inv, "compact"); err != nil {
			return true, fmt.Errorf("claude-mock: SessionStart (compact) hook blocked: %w", err)
		}
		return true, nil

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

// writeCompactBoundary writes the record real Claude Code opens a compaction
// with: parentless — the context before it is gone — and naming, as its
// logicalParentUuid, the last record before the compaction (or logicalParent,
// when a scenario overrides it). compactMetadata.preservedMessages lists the
// records the compaction kept, which a fork of the compacted session copies in
// after the boundary (forkTranscript).
//
// It is APPENDED to the file the session is writing, not a new file: measured,
// the file a real compaction happened in holds its boundary part-way down, and
// only a later resume moves the conversation to a new file.
func writeCompactBoundary(tr *transcript, logicalParent string, preserve int) {
	last := tr.lastUUID()
	if logicalParent == "" {
		logicalParent = last
	}
	preserved := []string{}
	if preserve > 0 && last != "" {
		preserved = append(preserved, last)
	}
	tr.persistMap(map[string]any{
		"parentUuid":        nil,
		"logicalParentUuid": logicalParent,
		"type":              "system",
		"subtype":           "compact_boundary",
		"content":           "Conversation compacted",
		"isMeta":            false,
		"level":             "info",
		"compactMetadata": map[string]any{
			"trigger":           "auto",
			"preservedMessages": map[string]any{"uuids": preserved},
		},
	})
}
