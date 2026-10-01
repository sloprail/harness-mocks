package tools

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOutcomeOf(t *testing.T) {
	assert.Equal(t, ReadEmpty, Read("", 0, 0).OutcomeOf(""))
	assert.Equal(t, ReadPastEnd, Read("a\nb", 9, 0).OutcomeOf("a\nb"))
	assert.Equal(t, ReadLines, Read("a\nb", 2, 0).OutcomeOf("a\nb"))
}

func TestRefusedEdit(t *testing.T) {
	r, ok := RefusedEdit("abc", "x", false)
	assert.True(t, ok)
	assert.Equal(t, Refusal{Absent: true}, r)
	r, ok = RefusedEdit("aa", "a", false)
	assert.True(t, ok)
	assert.Equal(t, Refusal{Matches: 2}, r)
	_, ok = RefusedEdit("aa", "a", true)
	assert.False(t, ok)
	_, ok = RefusedEdit("ab", "a", false)
	assert.False(t, ok)
}

func TestReadFileOfADirectory(t *testing.T) {
	_, err := ReadFile(t.TempDir())
	assert.ErrorIs(t, err, ErrIsDir)
}

func TestGlobInputRefused(t *testing.T) {
	assert.True(t, GlobInputRefused("a\x00b", ""))
	assert.True(t, GlobInputRefused("*", "d\x00"))
	assert.False(t, GlobInputRefused("*.go", "src"))
}
