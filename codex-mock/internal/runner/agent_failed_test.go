package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sloprail/harness-mocks/codex-mock/internal/events"
	"github.com/sloprail/harness-mocks/codex-mock/internal/hooks"
	"github.com/sloprail/harness-mocks/internal/tasks"
	"github.com/sloprail/harness-mocks/internal/toolcall"
	"github.com/sloprail/harness-mocks/internal/toolspec"
)

// A sub-agent whose start fails (its rollout cannot be created) is not told of
// as an empty completion, and no status is invented for it: the mock says so on
// stderr when it happens, and a wait that names it fails with a clear error.
func TestAFailedSubAgentStartFailsTheWaitFast(t *testing.T) {
	notAHome := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(notAHome, nil, 0o644)) // CODEX_HOME that is a file: no rollout can be made under it
	var stderr bytes.Buffer
	reg := tasks.NewRegistry()
	defer reg.Shutdown()
	h := toolHost{state: &state{
		cfg: Config{CodexHome: notAHome, Cwd: t.TempDir(), Stderr: &stderr}, id: "main", home: notAHome, refused: &toolspec.Refusals{},
		hooks: &hooks.Invoker{}, events: events.New(&bytes.Buffer{}), bg: reg,
	}}
	h.startBackground(toolcall.Call{Input: []byte(`{"message":"m"}`)}, `{"agent_id":"sub-1","nickname":"x"}`)
	task := reg.Find("sub-1")
	require.NotNil(t, task)
	select {
	case <-task.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("the sub-agent never ended")
	}
	assert.Contains(t, task.Failure, "failed to start the sub-agent")
	assert.Contains(t, stderr.String(), "sub-agent sub-1")
	assert.Empty(t, task.Result)

	in, _ := json.Marshal(map[string]any{"targets": []string{"sub-1"}, "timeout_ms": 10000})
	res := h.waitAgent(context.Background(), toolcall.Call{Input: in})
	assert.True(t, res.Failed)
	assert.Contains(t, res.Output, "sub-agent sub-1 failed")
	assert.Contains(t, res.Output, "refuses it rather than inventing it")
}
