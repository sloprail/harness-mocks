package toolexec

import (
	"encoding/json"

	"github.com/sloprail/harness-mocks/internal/tools"
)

// askInput is the argument shape for the AskUserQuestion tool: the questions the model asked, and the
// answers a PreToolUse hook added to them in its updatedInput (the way a run with no terminal has them).
// sr:docs https://code.claude.com/docs/en/hooks#askuserquestion
type askInput struct {
	Questions []struct {
		Question    string `json:"question"`
		MultiSelect bool   `json:"multiSelect"`
		Options     []struct {
			Label string `json:"label"`
		} `json:"options"`
	} `json:"questions"`
	Answers map[string]string `json:"answers"`
}

// executeAskUserQuestion answers an AskUserQuestion the way claude 2.1.285 does (recorded:
// snapshots/runs/ask-user-question-tool): the agent is told the answers, and the structured result is the
// input as it stands with the answers in it. The mock has no user to ask: the answers are the ones a
// PreToolUse hook gave the call, and a call with none is an error result saying so.
//
// sr:provides ask-user-question-tool/claude
func executeAskUserQuestion(raw json.RawMessage) Result {
	var inp askInput
	if err := json.Unmarshal(raw, &inp); err != nil || len(inp.Questions) == 0 {
		return Result{Output: "AskUserQuestion: missing or invalid 'questions' field", IsError: true}
	}
	if len(inp.Answers) == 0 {
		return Result{Output: "AskUserQuestion: the mock has no user to ask: a PreToolUse hook has to answer it, with an updatedInput that holds the questions and their answers", IsError: true}
	}
	var questions []tools.Question
	for _, q := range inp.Questions {
		asked := tools.Question{Question: q.Question, MultiSelect: q.MultiSelect}
		for _, o := range q.Options {
			asked.Options = append(asked.Options, o.Label)
		}
		questions = append(questions, asked)
	}
	var structured map[string]any
	_ = json.Unmarshal(raw, &structured)
	return Result{Output: tools.AnswerText(questions, inp.Answers), ToolUseResult: structured}
}
