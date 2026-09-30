package runner

import (
	"context"
	"fmt"
	"time"

	"github.com/sloprail/harness-mocks/claude-mock/internal/hooks"
)

// subagentRun is one dispatched sub-agent: where it runs, what it is, and the
// two transcripts involved — its own sidechain file, which its records go to,
// and the session's, which its hooks are told about.
type subagentRun struct {
	parent         Config
	subCwd         string
	agentID        string
	agentType      string
	sidechain      string
	parentReported string
	sessionFile    string
	script         string
	prompt         string
	background     bool
	spawnDepth     int
	toolUseID      string
	description    string
	outputFile     string
}

// subagentOutcome is how a sub-agent's run ended.
type subagentOutcome struct {
	finalText string
	failure   string
	toolUses  int
}

// execute fires SubagentStart, runs the sub-agent (re-running it while
// SubagentStop blocks) and returns how it ended.
func (s *subagentRun) execute(ctx context.Context, inv *hooks.Invoker, bg *backgroundTasks, prompt string) subagentOutcome {
	started := time.Now()
	writeFrame(s.parent, map[string]any{
		"type": "system", "subtype": "task_started", "task_id": s.agentID, "tool_use_id": s.toolUseID,
		"description": s.description, "subagent_type": s.agentType, "is_backgrounded": s.background,
		"spawn_depth": s.spawnDepth, "task_type": "local_agent", "prompt": prompt,
	})
	sideInv := s.invoker(inv)
	// SubagentStart — cannot block. transcript_path is the SESSION's (the
	// invoker's default); the sub-agent is named by agent_id.
	_, _ = sideInv.Fire(ctx, hooks.Input{
		SessionID:     s.parent.SessionID,
		Cwd:           s.subCwd,
		HookEventName: hooks.EventSubagentStart,
		AgentType:     s.agentType,
		AgentID:       s.agentID,
	})

	blockCap := stopHookBlockCap() // 0 = unlimited
	out := s.run(ctx, bg, prompt)
	for turn := 0; ; turn++ {
		// What the hook said is recorded as it fires — into the SUB-AGENT's own
		// file, where real Claude Code writes a SubagentStop's feedback — before
		// deciding whether to loop, so the last refusal before the cap is on the
		// record too.
		blocked, reason := fireSubagentStop(ctx, s, sideInv, bg, out.lastAssistant, turn > 0)
		if !blocked {
			break
		}
		if blockCap > 0 && turn >= blockCap {
			fmt.Fprintf(s.parent.Stderr, "claude-mock: SubagentStop still blocked after %d turns (cap) — giving up\n", blockCap)
			break
		}
		fmt.Fprintf(s.parent.Stderr, "claude-mock: SubagentStop blocked (%s) — re-running subagent (turn %d)\n", reason, turn+1)
		next := s.run(ctx, bg, prompt)
		next.toolUses += out.toolUses
		out = next
	}
	final := out.finalText
	if final == "" {
		final = out.lastAssistant
	}
	status, summary := "completed", final
	if out.failure != "" {
		status, summary = "failed", out.failure
	}
	writeFrame(s.parent, map[string]any{
		"type": "system", "subtype": "task_updated", "task_id": s.agentID,
		"patch": map[string]any{"status": status, "end_time": time.Now().UnixMilli()},
	})
	writeFrame(s.parent, map[string]any{
		"type": "system", "subtype": "task_notification", "task_id": s.agentID, "tool_use_id": s.toolUseID,
		"status": status, "output_file": s.outputFile, "summary": summary,
		"usage": map[string]any{"total_tokens": 0, "tool_uses": out.toolUses, "duration_ms": time.Since(started).Milliseconds()},
	})
	return subagentOutcome{finalText: final, failure: out.failure, toolUses: out.toolUses}
}

// runOutcome is one nested run of a sub-agent's script.
type runOutcome struct {
	finalText     string
	lastAssistant string
	failure       string
	toolUses      int
}

// invoker is inv recording into the sub-agent's own file — real Claude Code
// writes a sub-agent's SubagentStart and SubagentStop attachments there, never
// into the session's. The file is opened only when a hook actually ran, and
// closed again right after.
func (s *subagentRun) invoker(inv *hooks.Invoker) *hooks.Invoker {
	stamp := newRecordStamp(s.parent.SessionID, s.subCwd)
	stamp.IsSidechain, stamp.AgentID = true, s.agentID
	return inv.WithRecorder(func(in hooks.Input, runs []hooks.HandlerRun) {
		side, err := openTranscript(s.sidechain, s.parentReported, stamp)
		if err != nil {
			return
		}
		defer side.Close()
		side.recordHookRuns(in, runs)
	})
}
