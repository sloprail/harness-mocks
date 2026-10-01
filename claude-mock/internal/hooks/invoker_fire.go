package hooks

import (
	"context"
	"encoding/json"
	"fmt"

	corehooks "github.com/sloprail/harness-mocks/internal/hooks"
)

// Fire invokes every handler configured for the event and matcher and returns
// their merged Output. Real Claude Code runs all matching hooks of an event
// (docs: "All matching hooks run in parallel"), so a handler that blocks does
// not stop the others from running; the first block is returned, as a
// *BlockError, once they all have.
//
// Exit 2 is a block for every event here; what a block MEANS is the caller's
// to decide, because real Claude Code differs per event — SessionStart and
// SubagentStart treat it as a non-blocking error (docs, "Exit code 2
// behavior per event"), PreToolUse refuses the tool call, Stop re-prompts.
// sr:docs https://code.claude.com/docs/en/hooks#exit-code-2-behavior-per-event
func (inv *Invoker) Fire(ctx context.Context, input Input) (Output, error) {
	out, _, err := inv.FireRuns(ctx, input)
	return out, err
}

// FireRuns is Fire, also handing back each handler's run, for a caller that
// reports runs itself.
func (inv *Invoker) FireRuns(ctx context.Context, input Input) (Output, []HandlerRun, error) {
	// sr:provides hook-common-payload/claude
	common := corehooks.CommonFields(
		corehooks.Common{TranscriptPath: input.TranscriptPath, Cwd: input.Cwd, Agent: corehooks.Agent{ID: input.AgentID, Type: input.AgentType}},
		corehooks.Common{TranscriptPath: inv.transcriptPath, Cwd: inv.cwd},
		corehooks.Agent{ID: inv.agentID, Type: inv.agentType})
	input.TranscriptPath, input.Cwd = common.TranscriptPath, common.Cwd
	input.AgentID, input.AgentType = common.Agent.ID, common.Agent.Type
	handlers := inv.settings.EntriesFor(input.HookEventName, matcherSubject(input))
	if len(handlers) == 0 {
		return Output{}, nil, nil
	}

	payload, err := json.Marshal(input)
	if err != nil {
		return Output{}, nil, fmt.Errorf("hooks: marshal input: %w", err)
	}

	// The hook subprocess's OWN working directory must be THIS event's cwd, not the
	// Invoker's fixed construction-time cwd: a SubagentStart/Stop fired for a
	// isolation="worktree" subagent carries input.Cwd = the subagent's isolated worktree
	// (see agent.go's subCwd), and a hook command that shells out to a binary resolving
	// its identity from os.Getwd() must see that tree. Falls back to inv.cwd only for
	// an empty input.Cwd.
	hookCwd := input.Cwd
	if hookCwd == "" {
		hookCwd = inv.cwd
	}

	runs, outs := inv.runHandlers(ctx, handlers, input.HookEventName, hookCwd, payload)
	var merged Output
	var firstBlock error
	// sr:provides hooks-all-matching-run/claude
	acted, blocked := corehooks.ActedBlock(outs, strictExitEvents[input.HookEventName])
	for i, run := range runs {
		if run.Blocked {
			if blocked && i == acted {
				firstBlock = blockError(run)
			}
			continue
		}
		mergeOutput(&merged, run.Output)
	}
	if inv.recorder != nil && len(runs) > 0 {
		inv.recorder(input, runs)
	}
	return merged, runs, firstBlock
}

func mergeOutput(dst *Output, src Output) {
	if src.PlainText != "" {
		if dst.PlainText != "" {
			dst.PlainText += "\n"
		}
		dst.PlainText += src.PlainText
	}
	if src.Continue != nil {
		dst.Continue = src.Continue
	}
	if src.StopReason != "" {
		dst.StopReason = src.StopReason
	}
	if src.SystemMessage != "" {
		dst.SystemMessage = src.SystemMessage
	}
	// The winning decision brings its reason.
	if d := corehooks.StrongerDecision(dst.Decision, src.Decision); d != dst.Decision {
		dst.Decision, dst.Reason = d, src.Reason
	} else if dst.Reason == "" {
		dst.Reason = src.Reason
	}
	if src.HookSpecificOutput != nil {
		// Several hooks deciding one tool call: the stronger permission
		// decision wins, with its reason, whatever order they ran in (docs,
		// "PreToolUse decision control").
		h := *src.HookSpecificOutput
		if prev := dst.HookSpecificOutput; prev != nil {
			if corehooks.StrongerPermission(h.PermissionDecision, prev.PermissionDecision) != h.PermissionDecision {
				h.PermissionDecision, h.PermissionDecisionReason = prev.PermissionDecision, prev.PermissionDecisionReason
			}
			// Every hook's additionalContext reaches the agent, not the last's.
			h.AdditionalContext = corehooks.JoinContext(prev.AdditionalContext, h.AdditionalContext)
		}
		dst.HookSpecificOutput = &h
	}
}
