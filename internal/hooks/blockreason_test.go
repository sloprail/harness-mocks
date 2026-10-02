package hooks

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBlockReason(t *testing.T) {
	blocked, why := BlockReason(errors.New("[hook.sh]: refused"), "", "")
	assert.True(t, blocked)
	assert.Equal(t, "[hook.sh]: refused", why, "a block by status gives its text")

	blocked, why = BlockReason(nil, "block", "keep going")
	assert.True(t, blocked)
	assert.Equal(t, "keep going", why, "a block by decision gives the reason printed")

	blocked, why = BlockReason(errors.New("failed"), "block", "ignored")
	assert.True(t, blocked)
	assert.Equal(t, "failed", why, "status wins over what was printed")

	blocked, why = BlockReason(nil, "", "unused reason")
	assert.False(t, blocked)
	assert.Empty(t, why)
	blocked, _ = BlockReason(nil, "approve", "x")
	assert.False(t, blocked, "any decision but block lets the turn end")
}
