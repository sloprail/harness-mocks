package runner

import (
	"context"
	"fmt"
	"time"

	"github.com/sloprail/harness-mocks/claude-mock/internal/hooks"
	"github.com/sloprail/harness-mocks/internal/subagents"
	"github.com/sloprail/harness-mocks/internal/tasks"
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
	// cleanup removes an isolated sub-agent's clean worktree once it has finished
	// (subagents.Isolation.Cleanup); nil when it has no real worktree.
	cleanup func(context.Context) bool
}

// subagentOutcome is how a sub-agent's run ended.
type subagentOutcome struct {
	finalText string
	failure   string
	toolUses  int
}

// execute fires SubagentStart, runs the sub-agent (re-running it while
// SubagentStop blocks) and returns how it ended. The sequence, the block loop
// and its cap are the sub-agent core's (subagents.Execute); this supplies the
// hooks, the script run and Claude Code's task frames.
//
// sr:provides subagent-lifecycle-hooks/claude
// sr:provides subagent-stop-block-loop/claude
func (s *subagentRun) execute(ctx context.Context, inv *hooks.Invoker, bg *backgroundTasks, prompt string) subagentOutcome {
	started := time.Now()
	frames := frameObserver{s.parent}
	task := tasks.NewTask(tasks.Agent, s.agentID)
	task.ToolUseID, task.Description, task.AgentType, task.OutputFile = s.toolUseID, s.description, s.agentType, s.outputFile
	task.Meta = taskStart{
		ID: s.agentID, ToolUseID: s.toolUseID, Description: s.description, TaskType: "local_agent",
		Backgrounded: s.background, SubagentType: s.agentType, SpawnDepth: s.spawnDepth, Prompt: prompt,
	}
	tasks.Announce(task, frames)
	sideInv := s.invoker(inv)
	out := subagents.Execute(subagents.Hooks{
		// SubagentStart — cannot block. transcript_path is the SESSION's (the
		// invoker's default); the sub-agent is named by agent_id.
		Start: func() {
			_, _ = sideInv.Fire(ctx, hooks.Input{
				SessionID:     s.parent.SessionID,
				Cwd:           s.subCwd,
				HookEventName: hooks.EventSubagentStart,
				AgentType:     s.agentType,
				AgentID:       s.agentID,
			})
		},
		// What the hook said is recorded as it fires — into the SUB-AGENT's own
		// file, where real Claude Code writes a SubagentStop's feedback — so the
		// last refusal before the cap is on the record too.
		Stop: func(active bool, last string) (bool, string) {
			return fireSubagentStop(ctx, s, sideInv, bg, last, active)
		},
		OnRerun: func(reason string, turn int) {
			fmt.Fprintf(s.parent.Stderr, "claude-mock: SubagentStop blocked (%s) — re-running subagent (turn %d)\n", reason, turn)
		},
		OnCap: func(blockCap int) {
			fmt.Fprintf(s.parent.Stderr, "claude-mock: SubagentStop still blocked after %d turns (cap) — giving up\n", blockCap)
		},
	}, stopHookBlockCap(), func() subagents.Outcome { return s.run(ctx, bg, prompt) })
	final := out.FinalText
	if final == "" {
		final = out.LastAssistant
	}
	task.Result, task.Failure = final, out.Failure
	task.ToolUses, task.DurationMs = out.ToolUses, time.Since(started).Milliseconds()
	tasks.Conclude(task, frames)
	return subagentOutcome{finalText: final, failure: out.Failure, toolUses: out.ToolUses}
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
