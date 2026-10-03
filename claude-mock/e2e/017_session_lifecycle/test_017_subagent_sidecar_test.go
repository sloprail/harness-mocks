package e2e

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordedSub struct {
	meta map[string]any
	recs []rec
}

// recordedSubagents reads snapshots/runs/meta's sub-agent files by their
// sidecar's description: each sidecar and the sub-agent file's records.
func recordedSubagents(t *testing.T) map[string]recordedSub {
	t.Helper()
	metas, err := filepath.Glob("../../snapshots/runs/meta/samples/*/transcript/*/subagents/agent-*.meta.json")
	require.NoError(t, err)
	require.Len(t, metas, 3, "the meta run recorded outer, inner and iso")
	out := map[string]recordedSub{}
	for _, m := range metas {
		raw, err := os.ReadFile(m)
		require.NoError(t, err)
		var meta map[string]any
		require.NoError(t, json.Unmarshal(raw, &meta))
		out[meta["description"].(string)] = recordedSub{meta: meta, recs: readRecs(t, strings.TrimSuffix(m, ".meta.json")+".jsonl")}
	}
	return out
}

func keysOf(m map[string]any) []string {
	var ks []string
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

func promptOf(t *testing.T, r rec) string {
	t.Helper()
	var m struct {
		Content string `json:"content"`
	}
	require.NoError(t, json.Unmarshal(r.Message, &m))
	return m.Content
}

func runGitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	b, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	require.NoError(t, err, string(b))
}

// TestT017_66_SubagentSidecarAndFirstRecordAsRecorded replays the meta run's
// shape (an outer sub-agent that dispatches an inner one, and a sub-agent
// isolated in a worktree it leaves clean) and holds the mock's sub-agent files
// to what the real ones are: one file and one .meta.json sidecar per sub-agent
// beside the session's, the sidecar naming type, description, the spawning
// call's tool_use id, the depth, and for a nested one its parent; the clean
// isolated one's sidecar carrying worktreeCleanlyRemoved; and each file opening
// on a null-parent sidechain user record holding the dispatching prompt, the
// session's id and the sub-agent's agentId.
// sr:proves subagent-transcripts/claude
func TestT017_66_SubagentSidecarAndFirstRecordAsRecorded(t *testing.T) {
	real := recordedSubagents(t)
	wantPrompt := map[string]string{
		"outer": "Use the Agent tool (not in background) with subagent_type general-purpose, description inner, prompt Reply with the single word INNER. Then reply with the single word OUTER.",
		"inner": "Reply with the single word INNER.",
		"iso":   "Reply with the single word ISO",
	}
	for d, p := range wantPrompt {
		first := real[d].recs[0]
		assert.Nil(t, first.ParentUUID, "recorded %s: the file opens on a null-parent record", d)
		assert.True(t, first.IsSidechain)
		assert.Equal(t, "user", first.Type)
		assert.Equal(t, p, promptOf(t, first), "recorded %s: the first record is the dispatching prompt", d)
	}

	dir := t.TempDir()
	runGitIn(t, dir, "init", "-q")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "f"), []byte("x"), 0o644))
	runGitIn(t, dir, "add", "-A")
	runGitIn(t, dir, "-c", "user.email=t@t", "-c", "user.name=t", "commit", "-qm", "init")
	cfg := filepath.Join(dir, "config")
	inner := script(t, dir, "inner")
	outer := script(t, dir, "outer", toolUse("out1", "Agent", `{"prompt":"inner prompt","description":"inner","script":"`+inner+`"}`))
	iso := script(t, dir, "iso")
	root := script(t, dir, "root",
		toolUse("r1", "Agent", `{"prompt":"outer prompt","description":"outer","script":"`+outer+`"}`),
		toolUse("r2", "Agent", `{"prompt":"iso prompt","description":"iso","isolation":"worktree","script":"`+iso+`"}`),
	)
	out, code := runInDir(t, dir, nil, "--script", root, "--session-id", "sc-1",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)

	main := transcriptPath(t, cfg, dir, "sc-1")
	spawn := map[string]string{} // description -> spawning tool_use id, from the session's own file
	for _, r := range readRecs(t, main) {
		assert.False(t, r.IsSidechain, "no sub-agent record in the session's file")
		if r.Type != "assistant" || !strings.Contains(r.Raw, `"tool_use"`) {
			continue
		}
		var m struct {
			Content []struct {
				ID    string `json:"id"`
				Name  string `json:"name"`
				Input struct {
					Description string `json:"description"`
				} `json:"input"`
			} `json:"content"`
		}
		require.NoError(t, json.Unmarshal(r.Message, &m))
		for _, c := range m.Content {
			if c.Name == "Agent" {
				spawn[c.Input.Description] = c.ID
			}
		}
	}
	require.Len(t, spawn, 2, "the session's file holds the outer and iso calls; the nested call is in the outer's own file")

	metas, err := filepath.Glob(filepath.Join(strings.TrimSuffix(main, ".jsonl"), "subagents", "agent-*.meta.json"))
	require.NoError(t, err)
	require.Len(t, metas, 3, "one sidecar per sub-agent, beside the session's file")
	got := map[string]map[string]any{}
	ids := map[string]string{}
	for _, m := range metas {
		raw, err := os.ReadFile(m)
		require.NoError(t, err)
		var meta map[string]any
		require.NoError(t, json.Unmarshal(raw, &meta))
		desc := meta["description"].(string)
		got[desc] = meta
		id := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(m), "agent-"), ".meta.json")
		ids[desc] = id

		assert.Equal(t, keysOf(real[desc].meta), keysOf(meta), "%s: the sidecar's fields are the recorded ones", desc)
		assert.Equal(t, "general-purpose", meta["agentType"])
		assert.Equal(t, real[desc].meta["requestShape"], meta["requestShape"])
		assert.Equal(t, real[desc].meta["spawnDepth"], meta["spawnDepth"], "%s: depth as recorded", desc)
		assert.Equal(t, real[desc].meta["worktreeCleanlyRemoved"], meta["worktreeCleanlyRemoved"], "%s", desc)

		recs := readRecs(t, strings.TrimSuffix(m, ".meta.json")+".jsonl")
		first, _ := firstRoot(recs)
		require.NotEmpty(t, first.UUID, "%s: the file has a root record", desc)
		assert.Equal(t, "user", first.Type)
		assert.True(t, first.IsSidechain)
		assert.Equal(t, id, first.AgentID)
		assert.Equal(t, "sc-1", first.SessionID)
		assert.Equal(t, desc+" prompt", promptOf(t, first), "%s: the first record carries the dispatching prompt", desc)
	}
	require.Len(t, got, 3)
	assert.Equal(t, spawn["outer"], got["outer"]["toolUseId"], "the sidecar names the call that spawned it")
	assert.Equal(t, spawn["iso"], got["iso"]["toolUseId"])
	assert.Equal(t, ids["outer"], got["inner"]["parentAgentId"], "a nested sub-agent's sidecar names its parent")
	assert.NotContains(t, got["outer"], "parentAgentId")
	assert.True(t, strings.HasPrefix(got["inner"]["toolUseId"].(string), "out1"), "the nested sidecar names the outer's call")
	assert.Equal(t, true, got["iso"]["worktreeCleanlyRemoved"])
}
