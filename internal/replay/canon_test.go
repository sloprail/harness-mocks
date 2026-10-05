package replay

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// A masked key stays in the line with a placeholder for its value, whatever the
// value's type; a dropped key is gone.
func TestMaskKeepsTheKeyDropRemovesIt(t *testing.T) {
	c := New(Rules{MaskKeys: []string{"usage"}, DropKeys: []string{"script"}})
	got := c.Lines([]map[string]any{{"usage": map[string]any{"n": 1}, "script": "x", "k": "v"}})
	assert.Equal(t, []string{`{"k":"v","usage":"\u003cusage\u003e"}`}, got)
	other := New(Rules{MaskKeys: []string{"usage"}}).Lines([]map[string]any{{"k": "v"}})
	assert.NotEqual(t, got, other, "a line without the masked key differs")
}
