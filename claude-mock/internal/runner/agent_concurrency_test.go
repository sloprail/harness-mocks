package runner

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/sloprail/harness-mocks/internal/subagents"
)

// The limit is the harness's CLAUDE_CODE_MAX_CONCURRENT_SUBAGENTS when it is a positive number
// (recorded: runs/bgagent-concurrent-limit), else the default.
func TestConcurrentLimitReadsTheEnvironment(t *testing.T) {
	t.Setenv("CLAUDE_CODE_MAX_CONCURRENT_SUBAGENTS", "")
	assert.Equal(t, subagents.DefaultConcurrentLimit, concurrentLimit())
	t.Setenv("CLAUDE_CODE_MAX_CONCURRENT_SUBAGENTS", "1")
	assert.Equal(t, 1, concurrentLimit())
	t.Setenv("CLAUDE_CODE_MAX_CONCURRENT_SUBAGENTS", "0")
	assert.Equal(t, subagents.DefaultConcurrentLimit, concurrentLimit())
	t.Setenv("CLAUDE_CODE_MAX_CONCURRENT_SUBAGENTS", "many")
	assert.Equal(t, subagents.DefaultConcurrentLimit, concurrentLimit())
}
