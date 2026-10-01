package e2e

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// promptRecords is the human prompt records of a transcript: user records whose
// content is the plain text of a prompt (a tool_result's content is a block list).
func promptRecords(t *testing.T, recs []rec) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, r := range recs {
		var m map[string]any
		require.NoError(t, json.Unmarshal([]byte(r.Raw), &m))
		if msg, _ := m["message"].(map[string]any); m["type"] == "user" && msg != nil {
			if _, plain := msg["content"].(string); plain {
				out = append(out, m)
			}
		}
	}
	return out
}

// TestT017_23b_NonInteractivePromptsAreMarked: a prompt given to a `claude -p`
// run, a fresh session's and a resume's, is recorded with promptSource "sdk" and
// turnOrigin "sdk" (the -p prompt of the bgbash, midturn and bgagent runs), and
// every conversation record names the working directory it ran in.
// sr:proves transcript-record-envelope/claude
func TestT017_23b_NonInteractivePromptsAreMarked(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	out, code := runInDir(t, dir, nil, "--script", script(t, dir, "s"), "--session-id", "mk-1",
		"--project-dir", dir, "--config-dir", cfg, "-p", "first prompt")
	require.Equal(t, 0, code, out)
	out, code = runInDir(t, dir, nil, "--script", script(t, dir, "s"), "--resume", "mk-1",
		"--project-dir", dir, "--config-dir", cfg, "-p", "second prompt")
	require.Equal(t, 0, code, out)
	recs := readRecs(t, transcriptPath(t, cfg, dir, "mk-1"))
	prompts := promptRecords(t, recs)
	require.Len(t, prompts, 2, "the fresh prompt and the resume prompt")
	for i, text := range []string{"first prompt", "second prompt"} {
		assert.Equal(t, text, prompts[i]["message"].(map[string]any)["content"])
		assert.Equal(t, "sdk", prompts[i]["promptSource"], "prompt %d", i)
		assert.Equal(t, "sdk", prompts[i]["turnOrigin"], "prompt %d", i)
	}
	resolved, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	for _, r := range recs {
		if r.UUID == "" {
			continue
		}
		var m map[string]any
		require.NoError(t, json.Unmarshal([]byte(r.Raw), &m))
		assert.Equal(t, resolved, m["cwd"], "every conversation record names its working directory: %s", r.Raw)
	}
}
