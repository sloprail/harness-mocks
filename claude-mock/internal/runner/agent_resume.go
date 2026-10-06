package runner

import (
	"context"
	"encoding/json"
	"time"

	"github.com/sloprail/harness-mocks/claude-mock/internal/hooks"
	"github.com/sloprail/harness-mocks/claude-mock/internal/toolexec"
	"github.com/sloprail/harness-mocks/internal/tasks"
)

// sendMessageInput is what a SendMessage call names: the agent and the words (the harness fills
// recipient and content beside to and message; recorded: runs/fgsub-maxturns).
type sendMessageInput struct {
	To      string `json:"to"`
	Message string `json:"message"`
	Content string `json:"content"`
}

// resumeAgent answers a SendMessage to a sub-agent that has stopped: it goes on in the background from
// where it stopped, with the message as its prompt, and the call is answered at once (recorded:
// runs/fgsub-maxturns). The run starts once the call is answered, as a background launch does.
func (b *backgroundTasks) resumeAgent(cfg Config, inv *hooks.Invoker, toolUseID string, raw json.RawMessage) (toolexec.Result, func(answered <-chan struct{}) <-chan struct{}) {
	var in sendMessageInput
	_ = json.Unmarshal(raw, &in)
	words := in.Message
	if words == "" {
		words = in.Content
	}
	prior := b.agents.find(in.To)
	if prior == nil {
		return toolexec.Result{IsError: true, Failed: true, Output: "Error: no agent with id " + in.To + " to send a message to", ToolUseResult: "Error: no agent with id " + in.To}, nil
	}
	sub := *prior // the run that stopped, resumed: same agent, same transcript, a turn limit of its own again
	sub.launchedBy, sub.toolUseID, sub.prompt = prior.toolUseID, toolUseID, words
	sub.background, sub.announced, sub.startAnnounced, sub.begun = true, true, false, nil
	sub.limit = definitionTurnLimit(cfg, prior.agentType)
	sub.parent = cfg

	task := tasks.NewTask(tasks.Agent, sub.agentID)
	task.ToolUseID, task.Owner, task.Description, task.AgentType, task.OutputFile = toolUseID, cfg.AgentID, sub.description, sub.agentType, sub.outputFile
	pin := map[string]any{"id": sub.agentID, "name": sub.agentID, "ref": randomID(6)}
	result := map[string]any{"success": true, "message": "Resuming agent " + sub.agentID[:7], "resumedAgentId": sub.agentID, "pin": pin}
	text, _ := json.Marshal(orderedMessage{true, result["message"].(string), sub.agentID, pin})
	res := toolexec.Result{Output: string(text), ContentAsBlocks: true, ToolUseResult: result}
	start := func(answered <-chan struct{}) <-chan struct{} {
		defer sub.announce(b, words) // after the task is registered: the running set names it
		b.StartAgent(task, func(ctx context.Context) {
			<-answered
			out := sub.execute(ctx, inv, b, words)
			task.Result, task.Failure = out.finalText, out.failure
			if sub.limit.Reached() {
				task.StoppedAtTurns = sub.limit.Max
			}
			task.ToolUses, task.DurationMs = out.toolUses, time.Since(task.Started).Milliseconds()
			if out.failure != "" {
				task.ExitCode = 1
			}
		})
		return nil
	}
	return res, start
}

// orderedMessage is the answer's text with its keys in the order the harness writes them.
type orderedMessage struct {
	Success        bool           `json:"success"`
	Message        string         `json:"message"`
	ResumedAgentID string         `json:"resumedAgentId"`
	Pin            map[string]any `json:"pin"`
}

// frameParent is the call the sub-agent's own frames name as their parent.
func (s *subagentRun) frameParent() string {
	if s.launchedBy != "" {
		return s.launchedBy
	}
	return s.toolUseID
}

// agentSteps is what the sub-agent's gates are read against: made with its first run, and the same
// ones when it goes on after a message, so its calls are counted from its first.
func (s *subagentRun) agentSteps() *agentSteps {
	if s.steps == nil {
		s.steps = newAgentSteps(s.parent.steps)
	}
	return s.steps
}
