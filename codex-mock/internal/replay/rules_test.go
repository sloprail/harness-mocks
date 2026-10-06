package replay

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	core "github.com/sloprail/harness-mocks/internal/replay"
)

// A value that differs per run and is about a cell's presence (a working
// directory, a transcript path, a duration) is replaced by a placeholder, the
// key stays: a payload that lacks it is a difference.
func TestPerRunValuesKeepTheirKey(t *testing.T) {
	repo, root := "/tmp/r/repo", "/tmp/r"
	payload := map[string]any{
		"cwd":                   repo,
		"transcript_path":       root + "/home/.codex/sessions/2026/10/03/rollout-x.jsonl",
		"agent_transcript_path": root + "/home/.codex/sessions/2026/10/03/rollout-y.jsonl",
		"wall_time_seconds":     0.5,
		"model":                 "gpt-5.6-luna",
		"usage":                 map[string]any{"input_tokens": 12},
		"hook_event_name":       "Stop",
	}
	line := core.New(Rules(repo, root)).Lines([]map[string]any{payload})[0]
	for _, k := range []string{"cwd", "transcript_path", "agent_transcript_path", "wall_time_seconds", "model", "usage"} {
		assert.True(t, strings.Contains(line, `"`+k+`":`), "%s dropped: %s", k, line)
	}
	assert.NotContains(t, line, repo)
	assert.NotContains(t, line, root)
	assert.NotContains(t, line, "gpt-5.6-luna")

	// a payload without one of them is not the same line
	delete(payload, "transcript_path")
	assert.NotEqual(t, line, core.New(Rules(repo, root)).Lines([]map[string]any{payload})[0])
}
