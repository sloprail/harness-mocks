package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// toolResultsOf is the text of each tool_result block in a transcript file, with
// whether it is an error.
func toolResultsOf(t *testing.T, path string) (texts []string, errs []bool) {
	t.Helper()
	for _, r := range readRecs(t, path) {
		if r.Type != "user" || !strings.Contains(r.Raw, `"tool_result"`) {
			continue
		}
		var m struct {
			Content []struct {
				Type    string          `json:"type"`
				Content json.RawMessage `json:"content"`
				IsError bool            `json:"is_error"`
			} `json:"content"`
		}
		if json.Unmarshal(r.Message, &m) != nil {
			continue
		}
		for _, c := range m.Content {
			if c.Type != "tool_result" {
				continue
			}
			var s string
			if json.Unmarshal(c.Content, &s) != nil {
				s = string(c.Content)
			}
			texts = append(texts, s)
			errs = append(errs, c.IsError)
		}
	}
	return
}

// TestT017_70_ForkAtTheDepthLimitIsRefusedByAnError replays the nested-fork-limit
// run (CLAUDE_CODE_MAX_SUBAGENT_SPAWN_DEPTH=1, a fork that tries to dispatch a
// sub-agent): the fork runs in the background, its sidecar says agentType
// "fork", isFork true, model "inherit"; it keeps its Agent tool, and the
// dispatch returns the error the real harness gave (nesting limit reached,
// depth 1 of 1) instead of "No such tool"; no sub-agent starts below it. A
// non-fork sub-agent at the same limit still has no Agent tool.
// sr:proves nested-subagents/claude
func TestT017_70_ForkAtTheDepthLimitIsRefusedByAnError(t *testing.T) {
	realSide := recordedFile(t, "../../snapshots/runs/nested-fork-limit/samples/*/transcript/*/subagents/agent-*.jsonl")
	realTexts, realErrs := toolResultsOf(t, realSide)
	require.NotEmpty(t, realTexts)
	assert.True(t, realErrs[len(realErrs)-1], "recorded: the fork's last tool result is an error")
	realRefusal := realTexts[len(realTexts)-1]
	require.True(t, strings.HasPrefix(realRefusal, "Subagent nesting limit reached (depth 1 of 1)."), realRefusal)
	rawMeta, err := os.ReadFile(strings.TrimSuffix(realSide, ".jsonl") + ".meta.json")
	require.NoError(t, err)
	var realMeta map[string]any
	require.NoError(t, json.Unmarshal(rawMeta, &realMeta))
	assert.Equal(t, "fork", realMeta["agentType"])
	assert.Equal(t, true, realMeta["isFork"])

	for _, kind := range []string{"fork", "general-purpose"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			cfg := filepath.Join(dir, "config")
			log := filepath.Join(dir, "starts.log")
			settings(t, dir, map[string]string{"SubagentStart": payloadLogger(t, dir, "log.sh", log, "")})
			inner := script(t, dir, "inner")
			mid := script(t, dir, "mid", toolUse("m1", "Agent", `{"prompt":"deeper","description":"inner","subagent_type":"general-purpose","script":"`+inner+`"}`))
			root := script(t, dir, "root", toolUse("r1", "Agent", `{"prompt":"layer","description":"outerfork","subagent_type":"`+kind+`","script":"`+mid+`"}`))
			out, code := runInDir(t, dir, []string{"CLAUDE_CODE_MAX_SUBAGENT_SPAWN_DEPTH=1"}, "--script", root, "--session-id", "fl-"+kind,
				"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
			require.Equal(t, 0, code, out)

			sides, err := filepath.Glob(filepath.Join(cfg, "projects", "*", "fl-"+kind, "subagents", "agent-*.jsonl"))
			require.NoError(t, err)
			require.Len(t, sides, 1, "no sub-agent starts below the one at the limit")
			texts, errs := toolResultsOf(t, sides[0])
			require.NotEmpty(t, texts)
			assert.True(t, errs[len(errs)-1])
			raw, err := os.ReadFile(strings.TrimSuffix(sides[0], ".jsonl") + ".meta.json")
			require.NoError(t, err)
			var meta map[string]any
			require.NoError(t, json.Unmarshal(raw, &meta))
			if kind == "fork" {
				assert.Equal(t, realRefusal, texts[len(texts)-1], "the refusal is the recorded one")
				assert.Equal(t, "fork", meta["agentType"])
				assert.Equal(t, true, meta["isFork"])
				assert.Equal(t, "inherit", meta["model"])
				assert.Equal(t, realMeta["requestShape"], meta["requestShape"], "a fork runs in the background")
				assert.Equal(t, keysOf(realMeta), keysOf(meta), "the sidecar's fields are the recorded ones")
				return
			}
			assert.Equal(t, "Error: No such tool available: Agent", texts[len(texts)-1], "a non-fork has no Agent tool at the limit")
			assert.NotContains(t, meta, "isFork")
		})
	}
}
