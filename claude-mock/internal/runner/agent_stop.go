package runner

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"unicode/utf16"

	"github.com/sloprail/harness-mocks/claude-mock/internal/hooks"
	corehooks "github.com/sloprail/harness-mocks/internal/hooks"
	"github.com/sloprail/harness-mocks/internal/subagents"
)

// run drives the sub-agent's script as a nested run writing its sidechain
// file, and reports its final text, its last assistant text, how many tools
// it called, and why it failed if it did.
func (s *subagentRun) run(ctx context.Context, bg *backgroundTasks, prompt string) subagents.Outcome {
	if s.script == "" {
		return subagents.Outcome{FinalText: "no subagent script"}
	}
	var buf bytes.Buffer
	subCfg := Config{
		ScriptPath:              s.script,
		SessionID:               s.parent.SessionID,
		AgentID:                 s.agentID,
		AgentType:               s.agentType,
		IsResume:                true,
		Prompt:                  prompt,
		Cwd:                     s.subCwd,
		ProjectDir:              s.parent.ProjectDir,
		ConfigDir:               s.parent.ConfigDir,
		PluginCacheDir:          s.parent.PluginCacheDir,
		Model:                   s.parent.Model,
		Stderr:                  s.parent.Stderr,
		Out:                     &buf,
		SuppressSubagentHooks:   true,
		SyncSubagent:            !s.background,
		SidechainPath:           s.sidechain,
		ParentTranscriptPath:    s.parentReported,
		bg:                      bg,
		wake:                    s.parent.wake,
		stream:                  s.parent.stream,
		sessionFile:             s.sessionFile,
		spawnDepth:              s.spawnDepth,
		SpawnLimit:              s.parent.SpawnLimit,
		BackgroundTasksDisabled: s.parent.BackgroundTasksDisabled,
	}
	out := subagents.Outcome{}
	if err := Run(subagents.WithLimit(ctx, s.limit), subCfg); err != nil {
		fmt.Fprintf(s.parent.Stderr, "claude-mock: subagent run error: %v\n", err)
		out.Failure = err.Error()
	}
	out.FinalText = lastResultText(buf.Bytes())
	out.LastAssistant = lastAssistantText(buf.Bytes())
	out.ToolUses = countToolUses(buf.Bytes())
	return out
}

// fireSubagentStop fires SubagentStop and reports whether it BLOCKED — by
// exiting 2 or by an exit-0 {"decision":"block"} — and why. The payload is the
// real one: the session's transcript_path, the sub-agent's own
// agent_transcript_path, agent_id/agent_type, the sub-agent's cwd (its
// isolated worktree under isolation="worktree"), stop_hook_active on every
// re-fire after a block, last_assistant_message, and the session's
// background_tasks and session_crons.
// sr:docs https://code.claude.com/docs/en/hooks#subagentstop
func fireSubagentStop(ctx context.Context, s *subagentRun, inv *hooks.Invoker, bg *backgroundTasks, lastAssistant string, stopHookActive bool) (bool, string) {
	active := stopHookActive
	facts := subagents.Stop(s.sidechain, lastAssistant, bg.Registry)
	running := backgroundTaskList(facts.Tasks)
	crons := sessionCrons(s.parent.wake)
	out, err := inv.Fire(ctx, hooks.Input{
		SessionID:            s.parent.SessionID,
		Cwd:                  s.subCwd,
		AgentTranscriptPath:  facts.TranscriptPath,
		HookEventName:        hooks.EventSubagentStop,
		AgentType:            s.agentType,
		AgentID:              s.agentID,
		StopHookActive:       &active,
		LastAssistantMessage: &facts.LastMessage,
		BackgroundTasks:      &running,
		SessionCrons:         &crons,
	})
	return corehooks.BlockReason(err, out.Decision, out.Reason)
}

// sectionHash is the binary's harnessSectionHash of a report's text blocks:
// the first 16 hex digits of sha256(len, then ":"+len16(text)+":"+text per
// block), lengths in UTF-16 code units as JavaScript counts them.
func sectionHash(blocks []map[string]any) string {
	h := sha256.New()
	h.Write([]byte(strconv.Itoa(len(blocks))))
	for _, b := range blocks {
		t, _ := b["text"].(string)
		h.Write([]byte(":" + strconv.Itoa(len(utf16.Encode([]rune(t)))) + ":"))
		h.Write([]byte(t))
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}
