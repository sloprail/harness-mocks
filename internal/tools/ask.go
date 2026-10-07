package tools

import (
	"slices"
	"strings"
)

// Question is one multiple-choice question put to the user: its text, and the labels of its options.
type Question struct {
	Question    string
	Options     []string
	MultiSelect bool
}

// AnswerText is what the agent is told once the user has answered: each question with the answer it got,
// in the order asked. When every answer is the label of an option (the labels of a multiple-choice one
// joined by a comma and a space) the questions "have been answered"; when any is the user's own words, the
// text is neutral and tells the agent to read the answers carefully, so that it follows what they say.
//
// sr:capability ask-user-question-tool
func AnswerText(questions []Question, answers map[string]string) string {
	var pairs []string
	chosen := true
	for _, q := range questions {
		a, ok := answers[q.Question]
		if !ok {
			continue
		}
		pairs = append(pairs, `"`+q.Question+`"="`+a+`"`)
		chosen = chosen && isChoice(q, a)
	}
	list := strings.Join(pairs, ", ")
	if chosen {
		return "Your questions have been answered: " + list + ". You can now continue with these answers in mind."
	}
	return "The user answered: " + list + ". Read the answers carefully — they may request clarification, changes, or that you not proceed — and follow what they actually say."
}

// isChoice reports whether the answer picks only the question's options.
func isChoice(q Question, answer string) bool {
	if !q.MultiSelect {
		return slices.Contains(q.Options, answer)
	}
	for _, label := range strings.Split(answer, ", ") {
		if !slices.Contains(q.Options, label) {
			return false
		}
	}
	return answer != ""
}
