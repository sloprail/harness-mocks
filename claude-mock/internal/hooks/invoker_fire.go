package hooks

import (
	"context"
	"encoding/json"
	"fmt"
	corehooks "github.com/sloprail/harness-mocks/internal/hooks"
	"log/slog"
	"time"
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

// matcherValue is what a hook's matcher is matched against: the compaction
// trigger ("manual" | "auto") for PreCompact and PostCompact, the tool name
// otherwise.
// sr:docs https://code.claude.com/docs/en/hooks#precompact
func matcherValue(in Input) string {
	if in.HookEventName == EventPreCompact || in.HookEventName == EventPostCompact {
		return in.Trigger
	}
	return in.ToolName
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
	handlers := inv.settings.EntriesFor(input.HookEventName, matcherValue(input))
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

	var merged Output
	var runs []HandlerRun
	var firstBlock error
	for _, h := range handlers {
		run, blockErr := inv.invoke(ctx, h, input.HookEventName, hookCwd, payload)
		runs = append(runs, run)
		if blockErr != nil {
			if firstBlock == nil {
				firstBlock = blockErr
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

func (inv *Invoker) invoke(ctx context.Context, h HandlerSpec, ev EventName, hookCwd string, payload []byte) (HandlerRun, error) {
	timeout := defaultHookTimeout
	if h.Timeout > 0 {
		timeout = time.Duration(h.Timeout) * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	switch h.Type {
	case "command":
		return inv.invokeCommand(ctx, h, ev, hookCwd, payload)
	case "http":
		out, err := inv.invokeHTTP(ctx, h, payload)
		return HandlerRun{Command: h.URL, Output: out}, err
	default:
		slog.Debug("hooks: unsupported handler type", "type", h.Type)
		return HandlerRun{}, nil
	}
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
