package runner

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"strings"
	"time"

	"github.com/sloprail/harness-mocks/claude-mock/internal/hooks"
	"github.com/sloprail/harness-mocks/claude-mock/internal/toolexec"
	"github.com/sloprail/harness-mocks/internal/subagents"
	"github.com/sloprail/harness-mocks/internal/tasks"
)

// launchAgent validates a background Agent call, prepares its sub-agent (its
// sidechain file, and the output-file symlink to it), and returns the receipt
// together with start, which runs the sub-agent concurrently. The caller
// starts it once the receipt's tool_result and PostToolUse are written;
// SubagentStart has fired by then, with the launch.
//
// sr:provides background-agent/claude
func (b *backgroundTasks) launchAgent(cfg Config, inv *hooks.Invoker, toolUseID string, raw json.RawMessage, tr *transcript) (toolexec.Result, func(answered <-chan struct{}) <-chan struct{}) {
	sub, in, errRes := prepareSubagent(b.Context(), cfg, inv, toolUseID, raw, tr, true)
	if sub == nil {
		return errRes, nil
	}
	outFile := sub.outputFile

	task := tasks.NewTask(tasks.Agent, sub.agentID)
	task.ToolUseID, task.Owner, task.Description, task.AgentType, task.OutputFile = toolUseID, cfg.AgentID, in.Description, sub.agentType, outFile

	model := in.Model
	if model == "" {
		model = cfg.Model
	}
	if model == "" {
		model = "default"
	}
	text := "Async agent launched successfully. (This tool result is internal metadata — never quote or paste any part of it, including the agentId below, into a user-facing reply.)\n" +
		"agentId: " + sub.agentID + " (internal ID - do not mention to user. Use SendMessage with to: '" + sub.agentID + "', summary: '<5-10 word recap>' to continue this agent.)\n" +
		"The agent is working in the background. You will be notified automatically when it completes. You know nothing about its results until that notification arrives — do not report, assume, or predict them; continue other work or respond to the user in the meantime.\n" +
		"Do not duplicate this agent's work — avoid working with the same files or topics it is using.\n" +
		"output_file: " + outFile + "\n" +
		"Do NOT Read or tail this file via the shell tool — it is the full subagent JSONL transcript and reading it will overflow your context. If the user asks for progress, say the agent is still running; you'll get a completion notification."
	res := toolexec.Result{
		Output:          text,
		ContentAsBlocks: true,
		ToolUseResult: map[string]any{
			"isAsync": true, "status": "async_launched", "agentId": sub.agentID,
			"description": in.Description, "prompt": in.Prompt, "outputFile": outFile,
			"canReadOutputFile": true, "resolvedModel": model,
		},
	}
	// The sub-agent is begun with the launch, ahead of the call's PostToolUse
	// (recorded: snapshots/runs/bgagent); its run starts after the answer.
	// One launched by a sub-agent is begun after the call's PostToolUse, with its run (recorded: runs/
	// bgagent-nested-launcher, the same order in every sample).
	if cfg.AgentID == "" && !in.StartAfterPost {
		sub.begun = subagents.Begin(sub.hooks(b.Context(), inv, b))
	} else if in.StartAfterPost {
		sub.began = make(chan struct{})
	}
	// The launch is announced (the running set, task_started) ahead of the call's result, as recorded
	// (runs/bgagent, bgagent-concurrent-limit, nested-fork-limit); the sub-agent's run waits for the answer.
	start := func(answered <-chan struct{}) <-chan struct{} {
		defer sub.announce(b, in.Prompt) // after the task is registered: the running set names it
		b.StartAgent(task, func(ctx context.Context) {
			<-answered
			out := sub.execute(ctx, inv, b, in.Prompt)
			b.stats.End(out.failure != "")
			sub.cleanupWorktree(ctx)
			task.Result, task.Failure = out.finalText, out.failure
			if sub.limit.Reached() {
				task.StoppedAtTurns = sub.limit.Max
			}
			task.ToolUses, task.DurationMs = out.toolUses, time.Since(task.Started).Milliseconds()
			if out.failure != "" {
				task.ExitCode = 1
			}
		})
		return sub.began
	}
	return res, start
}

// writeStreamLine writes one line to the output stream in a single Write, so
// a frame another goroutine writes (a background sub-agent's) can never land
// between a line and its newline.
func writeStreamLine(cfg Config, line []byte) {
	if len(line) == 0 {
		return
	}
	line = stampFrame(cfg, line)
	buf := make([]byte, 0, len(line)+1)
	buf = append(append(buf, line...), '\n')
	cfg.Out.Write(buf) //nolint:errcheck
	// a sub-agent's own messages also stream, to the session's stream, where the run's output is captured
	if cfg.AgentID != "" && cfg.stream != nil && isMessageFrame(line, cfg.background) {
		cfg.stream.Write(buf) //nolint:errcheck
	}
}

// inputValidationError is the tool_result real Claude Code returns for a call
// missing required parameters: "<tool_use_error>InputValidationError: <Tool>
// failed due to the following issue(s):\nThe required parameter `x` is
// missing</tool_use_error>", with the zod issues as toolUseResult.
func inputValidationError(tool string, missing []string) toolexec.Result {
	noun := "issue"
	if len(missing) > 1 {
		noun = "issues"
	}
	var lines []string
	var issues []map[string]any
	for _, m := range missing {
		lines = append(lines, "The required parameter `"+m+"` is missing")
		issues = append(issues, map[string]any{
			"expected": "string", "code": "invalid_type", "path": []string{m},
			"message": "Invalid input: expected string, received undefined",
		})
	}
	zod, _ := json.MarshalIndent(issues, "", "  ")
	return toolexec.Result{
		Output:        "<tool_use_error>InputValidationError: " + tool + " failed due to the following " + noun + ":\n" + strings.Join(lines, "\n") + "</tool_use_error>",
		IsError:       true,
		ToolUseResult: "InputValidationError: " + string(zod),
	}
}

// randomID is n lowercase alphanumerics, the shape of a real background task id.
func randomID(n int) string {
	const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789"
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return strings.Repeat("0", n)
	}
	for i := range buf {
		buf[i] = alphabet[int(buf[i])%len(alphabet)]
	}
	return string(buf)
}
