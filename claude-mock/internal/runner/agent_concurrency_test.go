// a10n:blueprint:ignore
package runner

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/sloprail/harness-mocks/internal/subagents"
)

// The limit is the one the entrypoint configured (CLAUDE_CODE_MAX_CONCURRENT_SUBAGENTS, a positive
// number; recorded: runs/bgagent-concurrent-limit), else the default.
func TestConcurrentLimitIsTheConfiguredOne(t *testing.T) {
	assert.Equal(t, subagents.DefaultConcurrentLimit, concurrentLimit(Config{}))
	assert.Equal(t, 1, concurrentLimit(Config{ConcurrentLimit: 1}))
	assert.Equal(t, subagents.DefaultConcurrentLimit, concurrentLimit(Config{ConcurrentLimit: -3}))
}
