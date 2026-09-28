package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
// It returns whether the compaction happened. The scenario drives it with
// {"type":"compact"[,"summary":…][,"trigger":"auto"|"manual"][,"preserve":N]
// [,"pre_tokens":N][,"post_tokens":N][,"preserved_segment":false][,"tail_offset":K]
// [,"logical_parent":…][,"id":…]}, or with its own
// isCompactSummary record, which is used as the summary.
// sr:docs https://code.claude.com/docs/en/hooks#precompact
// sr:docs https://code.claude.com/docs/en/hooks#postcompact
func compact(ctx context.Context, cfg Config, inv *hooks.Invoker, tr *transcript, rec *cliRecord, line []byte) bool {
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
	return true
}

// compactHookLines is what a compaction reports of its Pre/PostCompact
// hooks, one line per hook, in the 2.1.282 binary's wording:
// "<Event> [<command>] completed successfully[: <output>]" or
// "<Event> [<command>] failed[: <output>]".
func compactHookLines(event string, runs []hooks.HandlerRun) []string {
	var out []string
	for _, r := range runs {
		verdict, text := "completed successfully", strings.TrimSpace(r.Stdout)
		if r.ExitCode != 0 {
			verdict, text = "failed", strings.TrimSpace(r.Stderr)
		}
		l := event + " [" + r.Command + "] " + verdict
		if text != "" {
			l += ": " + text
		}
		out = append(out, l)
	}
	return out
}

// compactionSpec is what one compaction's boundary records.
type compactionSpec struct {
	logicalParent string
	preserve      int
	anchor        string
	trigger       string
	preTokens     int
	postTokens    int
	durationMs    int64
	withSegment   bool
	tailOffset    int
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

// writeCompactBoundary appends the compact_boundary a compaction opens with.
// It is APPENDED to the file the session is writing, not a new file: the file
// a real compaction happened in holds its boundary part-way down (46 of the 65
// real boundaries), and only a later resume moves the conversation to a new
// file.
func writeCompactBoundary(tr *transcript, spec compactionSpec) {
	// Two shapes, both observed in manual compactions of claude 2.1.282:
	//
	//   - with a preserved segment (F:compact; 64 of 65 real boundaries):
	//     compactMetadata names the kept records — preservedMessages {anchorUuid,
	//     uuids, allUuids} and preservedSegment {headUuid, anchorUuid, tailUuid},
	//     uuids being N consecutive written records (see the tail below). allUuids is uuids plus records
	//     never written to the file: a strict superset in 44 of 65 real
	//     boundaries, every extra id unwritten.
	//   - without one (F:compact-nohooks; 1 of 65): neither field.
	//
	// The logical parent is the preserved segment's TAIL — uuids' last id —
	// wherever it is written: all 55 real boundaries whose logical parent is a
	// written record. The tail sits in three places (66 real boundaries):
	//
	//   - the record written immediately before the boundary: 34 (33
	//     automatic, 1 manual), and F:compact-nohooks. This is the default.
	//   - an EARLIER written record, the segment ending 2 to 253 records
	//     before the boundary: 7 mid-file boundaries. "tail_offset": K ends the
	//     segment K records back.
	//   - a record copied in AFTER the boundary: the 13 fork files, and one
	//     mid-file boundary. forkTranscript writes this form.
	//
	// The remaining 11 real boundaries (all automatic) and the manual
	// F:compact name a record never written, which then closes allUuids:
	// "logical_parent":"unwritten". Any other logical_parent is used as given.
	withSegment := spec.withSegment
	kept := []string{}
	if withSegment {
		window := tr.lastUUIDs(spec.preserve + spec.tailOffset)
		if spec.tailOffset > 0 {
			if len(window) > spec.tailOffset {
				window = window[:len(window)-spec.tailOffset]
			} else {
				window = []string{}
			}
		}
		kept = window
	}
	logicalParent := spec.logicalParent
	switch logicalParent {
	case "":
		if len(kept) > 0 {
			logicalParent = kept[len(kept)-1]
		} else {
			logicalParent = tr.lastUUID()
		}
	case "unwritten":
		logicalParent = newRecordUUID()
	}
	all := append([]string(nil), kept...)
	if logicalParent != "" && !contains(kept, logicalParent) && !fileHasUUID(tr.path, logicalParent) {
		all = append(all, logicalParent)
	}
	meta := map[string]any{
		"trigger":    spec.trigger,
		"preTokens":  spec.preTokens,
		"durationMs": spec.durationMs,
	}
	if withSegment && len(kept) > 0 {
		meta["preservedSegment"] = map[string]any{
			"headUuid": kept[0], "anchorUuid": spec.anchor, "tailUuid": kept[len(kept)-1],
		}
		meta["preservedMessages"] = map[string]any{
			"anchorUuid": spec.anchor, "uuids": kept, "allUuids": all,
		}
	}
	meta["postTokens"] = spec.postTokens
	// cumulativeDroppedTokens: what the session's compactions have dropped so
	// far, this one's preTokens - postTokens included (both manual fixtures:
	// 23138-2308 = 20830, 22932-2334 = 20598). 2.1.282 always writes it; 13
	// of the 66 real boundaries, from older versions, lack it.
	meta["cumulativeDroppedTokens"] = lastCumulativeDropped(tr.path) + spec.preTokens - spec.postTokens
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

// lastCumulativeDropped is the cumulativeDroppedTokens of the last compact
// boundary in path, or 0.
func lastCumulativeDropped(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	last := 0
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.Contains(line, "compact_boundary") {
			continue
		}
		var rec struct {
			Subtype string `json:"subtype"`
			Meta    struct {
				Dropped int `json:"cumulativeDroppedTokens"`
			} `json:"compactMetadata"`
		}
		if json.Unmarshal([]byte(line), &rec) == nil && rec.Subtype == "compact_boundary" {
			last = rec.Meta.Dropped
		}
	}
	return last
}
