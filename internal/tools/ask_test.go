package tools

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAnswerText(t *testing.T) {
	qs := []Question{
		{Question: "Colour?", Options: []string{"Red", "Blue"}},
		{Question: "Sizes?", Options: []string{"S", "M", "L"}, MultiSelect: true},
	}
	chosen := AnswerText(qs, map[string]string{"Colour?": "Blue", "Sizes?": "S, M"})
	assert.Equal(t, `Your questions have been answered: "Colour?"="Blue", "Sizes?"="S, M". You can now continue with these answers in mind.`, chosen)
	for _, answers := range []map[string]string{
		{"Colour?": "teal", "Sizes?": "S"}, {"Colour?": "Red", "Sizes?": "S, XL"}, {"Colour?": "Red", "Sizes?": ""}, {"Colour?": "Red, Blue", "Sizes?": "S"},
	} {
		assert.Contains(t, AnswerText(qs, answers), "The user answered: ", answers)
	}
}
