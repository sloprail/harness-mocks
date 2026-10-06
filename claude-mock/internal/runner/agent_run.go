package runner

import (
	"context"
	"time"

	"github.com/sloprail/harness-mocks/claude-mock/internal/hooks"
	"github.com/sloprail/harness-mocks/internal/subagents"
	"github.com/sloprail/harness-mocks/internal/tasks"
)

// subagentRun is one dispatched sub-agent: where it runs, what it is, and the
// two transcripts involved — its own sidechain file, which its records go to,
// and the session's, which its hooks are told about.
type subagentRun struct {
	announced      bool // the sub-agent's prompt has been streamed: a re-run after a blocked stop does not stream it again
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
	// branch is the branch of an isolated sub-agent's git worktree, when it has one.
	branch string
	// limit is the sub-agent's turn limit (its definition's maxTurns), nil when none.
	limit *subagents.TurnLimit
	// cleanup removes an isolated sub-agent's clean worktree once it has finished
	// (subagents.Isolation.Cleanup); nil when it has no real worktree.
	cleanup func(context.Context) bool
	// begun is the sub-agent as begun at its launch (subagents.Begin), when it
	// was begun before its run; nil begins it with the run.
	begun func(blockCap int, run func() subagents.Outcome) subagents.Outcome
	// startFrames is the SubagentStart hook's frames that are still to be written (hook_frames).
	startFrames *pendingFrames
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
// sr:provides stop-block-cap/claude
func (s *subagentRun) execute(ctx context.Context, inv *hooks.Invoker, bg *backgroundTasks, prompt string) subagentOutcome {
	started := time.Now()
	frames := frameObserver{s.parent}
	task := tasks.NewTask(tasks.Agent, s.agentID)
	task.ToolUseID, task.Description, task.AgentType, task.OutputFile = s.toolUseID, s.description, s.agentType, s.outputFile
	task.Meta = taskStart{
		ID: s.agentID, ToolUseID: s.toolUseID, Description: s.description, TaskType: "local_agent",
		Backgrounded: s.background, SubagentType: s.agentType, SpawnDepth: s.spawnDepth, Prompt: prompt,
	}
	if !(s.background && s.begun != nil) { // a background sub-agent was announced with its launch
		tasks.Announce(bg.Registry, task, frames)
	}
	begin := s.begun
	if begin == nil {
		begin = subagents.Begin(s.hooks(ctx, inv, bg))
	}
	out := begin(stopHookBlockCap(), func() subagents.Outcome { return s.run(ctx, bg, prompt) })
	if !s.background { // only a foreground sub-agent's commands end with its response, after SubagentStop has listed them
		bg.EndOfResponse(s.agentID)
	}
	final := out.FinalText
	if final == "" {
		final = out.LastAssistant
	}
	task.Result, task.Failure = final, out.Failure
	task.ToolUses, task.DurationMs = out.ToolUses, time.Since(started).Milliseconds()
	tasks.Conclude(bg.Registry, task, frames)
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

// hooks are the hooks fired around the sub-agent.
func (s *subagentRun) hooks(ctx context.Context, inv *hooks.Invoker, bg *backgroundTasks) subagents.Hooks {
	sideInv := s.invoker(inv)
	return subagents.Limited(subagents.Hooks{
		// SubagentStart — cannot block. transcript_path is the SESSION's (the
		// invoker's default); the sub-agent is named by agent_id.
		Start: func() {
			in := hooks.Input{
				SessionID:     s.parent.SessionID,
				Cwd:           s.subCwd,
				HookEventName: hooks.EventSubagentStart,
				AgentType:     s.agentType,
				AgentID:       s.agentID,
			}
			_, runs, _ := sideInv.FireRuns(ctx, in)
			s.startFrames = startPending(s.parent, in, runs)
		},
		// What the hook said is recorded as it fires — into the SUB-AGENT's own
		// file, where real Claude Code writes a SubagentStop's feedback — so the
		// last refusal before the cap is on the record too.
		Stop: func(active bool, last string) (bool, string) {
			return fireSubagentStop(ctx, s, sideInv, bg, last, active)
		},
	}, s.limit)
}
