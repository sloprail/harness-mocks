package runner

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func jsonl(t *testing.T, recs ...map[string]any) []byte {
	t.Helper()
	var b strings.Builder
	for _, r := range recs {
		line, err := json.Marshal(r)
		require.NoError(t, err)
		b.Write(line)
		b.WriteByte('\n')
	}
	return []byte(b.String())
}

func uuidsOf(recs []map[string]any) []any {
	var out []any
	for _, r := range recs {
		out = append(out, r["uuid"])
	}
	return out
}

// sr:proves session-fork/claude
func TestForkSegment_Uncompacted(t *testing.T) {
	data := jsonl(t,
		map[string]any{"type": "custom-title"},
		map[string]any{"type": "user", "uuid": "u1", "parentUuid": nil},
		map[string]any{"type": "assistant", "uuid": "u2", "parentUuid": "u1"},
	)
	seg := forkSegment(data, "new")
	assert.Equal(t, []any{"u1", "u2"}, uuidsOf(seg), "the whole conversation, bookkeeping dropped")
	assert.Nil(t, seg[0]["parentUuid"])
	assert.Equal(t, "u1", seg[1]["parentUuid"], "parents unchanged")
}

// sr:proves session-fork/claude
func TestForkSegment_Compacted(t *testing.T) {
	boundary := map[string]any{
		"type": "system", "subtype": "compact_boundary", "uuid": "B", "parentUuid": nil, "logicalParentUuid": "p3",
		"compactMetadata": map[string]any{"preservedMessages": map[string]any{"uuids": []any{"p2", "p3"}}},
	}
	data := jsonl(t,
		map[string]any{"type": "user", "uuid": "p1", "parentUuid": nil},
		map[string]any{"type": "assistant", "uuid": "p2", "parentUuid": "p1"},
		map[string]any{"type": "user", "uuid": "p3", "parentUuid": "p2"},
		boundary,
		map[string]any{"type": "attachment", "uuid": "i", "parentUuid": "B"},
		map[string]any{"type": "user", "uuid": "S", "parentUuid": "i", "isCompactSummary": true},
		map[string]any{"type": "attachment", "uuid": "x", "parentUuid": "S"},
		map[string]any{"type": "assistant", "uuid": "y", "parentUuid": "x"},
	)
	seg := forkSegment(data, "new")
	assert.Equal(t, []any{"B", "i", "S", "p2", "p3", "x", "y"}, uuidsOf(seg),
		"boundary, what leads to the summary, the summary, the preserved records, then the rest")
	parents := []any{}
	for _, r := range seg {
		parents = append(parents, r["parentUuid"])
	}
	assert.Equal(t, []any{nil, "B", "i", "S", "p2", "p3", "x"}, parents, "one chain")
	assert.Equal(t, "p3", seg[0]["logicalParentUuid"], "the boundary is copied verbatim")
}

// sr:proves session-fork/claude
func TestForkSegment_LastBoundaryWins(t *testing.T) {
	b := func(u string, kept ...any) map[string]any {
		return map[string]any{"type": "system", "subtype": "compact_boundary", "uuid": u, "parentUuid": nil,
			"compactMetadata": map[string]any{"preservedMessages": map[string]any{"uuids": kept}}}
	}
	data := jsonl(t,
		map[string]any{"type": "user", "uuid": "a", "parentUuid": nil},
		b("B1", "a"),
		map[string]any{"type": "user", "uuid": "S1", "parentUuid": "B1", "isCompactSummary": true},
		map[string]any{"type": "assistant", "uuid": "c", "parentUuid": "S1"},
		b("B2", "c"),
		map[string]any{"type": "user", "uuid": "S2", "parentUuid": "B2", "isCompactSummary": true},
	)
	assert.Equal(t, []any{"B2", "S2", "c"}, uuidsOf(forkSegment(data, "new")))
}

// sr:proves session-fork/claude
func TestForkTranscript_RewritesSessionIDAndLeavesTheSourceAlone(t *testing.T) {
	cfg := t.TempDir()
	cwd := t.TempDir()
	src := sessionFilePath(cfg, cwd, "old")
	require.NoError(t, os.MkdirAll(filepath.Dir(src), 0o755))
	orig := jsonl(t, map[string]any{"type": "user", "uuid": "u1", "parentUuid": nil, "sessionId": "old"})
	require.NoError(t, os.WriteFile(src, orig, 0o644))
	dest := sessionFilePath(cfg, cwd, "new")
	require.NoError(t, forkTranscript(cfg, cwd, "old", dest, "new"))
	after, err := os.ReadFile(src)
	require.NoError(t, err)
	assert.Equal(t, orig, after)
	data, err := os.ReadFile(dest)
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	var last map[string]any
	require.NoError(t, json.Unmarshal([]byte(lines[len(lines)-1]), &last))
	assert.Equal(t, "u1", last["uuid"])
	assert.Equal(t, "new", last["sessionId"])
	assert.Error(t, forkTranscript(cfg, cwd, "old", dest, "new"), "a fork never overwrites")

	var noConv *ErrNoConversation
	err = forkTranscript(cfg, cwd, "missing", sessionFilePath(cfg, cwd, "n2"), "n2")
	require.True(t, errors.As(err, &noConv))
	assert.Equal(t, "No conversation found with session ID: missing", err.Error())
}

func TestFindSessionFile(t *testing.T) {
	cfg := t.TempDir()
	a := filepath.Join(cfg, "projects", "a", "s.jsonl")
	b := filepath.Join(cfg, "projects", "b", "s.jsonl")
	for _, p := range []string{a, b} {
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte("{}\n"), 0o644))
	}
	require.NoError(t, os.Chtimes(a, timeAgo(2), timeAgo(2)))
	assert.Equal(t, b, sessionFilePathIfExists(cfg, t.TempDir(), "s"), "the most recently written wins")
	require.NoError(t, os.MkdirAll(filepath.Join(cfg, "projects", "c", "d.jsonl"), 0o755))
	assert.Equal(t, "", sessionFilePathIfExists(cfg, t.TempDir(), "d"), "a directory is not a transcript")
	for _, bad := range []string{"", "..", "../x", "a/s"} {
		assert.Equal(t, "", sessionFilePathIfExists(cfg, t.TempDir(), bad), bad)
	}
}

func TestFileHasUUID(t *testing.T) {
	p := filepath.Join(t.TempDir(), "s.jsonl")
	require.NoError(t, os.WriteFile(p, []byte(`{"type":"user", "uuid": "root-1"}`+"\n"+`{"type":"user","parentUuid":"root-2","uuid":"x"}`+"\n"), 0o644))
	assert.True(t, fileHasUUID(p, "root-1"), "spacing does not matter")
	assert.False(t, fileHasUUID(p, "root-2"), "a parent is not the record's own uuid")
}

func timeAgo(hours int) time.Time { return time.Now().Add(-time.Duration(hours) * time.Hour) }

// sr:proves foreground-subagent-result/claude
func TestBuildAgentResult(t *testing.T) {
	sub := &subagentRun{agentID: "a0123456789abcdef", agentType: "general-purpose"}
	res := buildAgentResult(sub, agentToolInput{Prompt: "p", Model: "haiku"}, "", subagentOutcome{finalText: "line one\r\nline two", toolUses: 2}, 17, "")
	assert.True(t, res.ContentAsBlocks)
	assert.Equal(t, handbackFrame+"\n  line one\n  line two\nagentId: a0123456789abcdef (use SendMessage with to: 'a0123456789abcdef', summary: '<5-10 word recap>' to continue this agent)\n<usage>subagent_tokens: 0\ntool_uses: 2\nduration_ms: 17</usage>", res.Output)
	tur := res.ToolUseResult.(map[string]any)
	assert.Equal(t, "completed", tur["status"])
	assert.Equal(t, "haiku", tur["resolvedModel"])
	assert.Equal(t, []map[string]any{{"type": "text", "text": "line one\r\nline two"}}, tur["content"])

	wt := buildAgentResult(sub, agentToolInput{}, "", subagentOutcome{finalText: "x"}, 1, "/w/.claude/worktrees/agent-a")
	assert.Contains(t, wt.Output, "to continue this agent)\nworktreePath: /w/.claude/worktrees/agent-a\n<usage>")

	explore := buildAgentResult(&subagentRun{agentID: "a1", agentType: "Explore"}, agentToolInput{}, "", subagentOutcome{}, 1, "")
	assert.Equal(t, handbackFrame+"\n  (Subagent completed but returned no output.)", explore.Output, "Explore/Plan without a worktree: no trailer")
}

func TestSectionHashMatchesTheBinary(t *testing.T) {
	// claude 2.1.282 recorded harnessSectionHash db84e7ea1e7eb856 for a
	// sub-agent that replied HELPED.
	assert.Equal(t, "db84e7ea1e7eb856", sectionHash([]map[string]any{{"type": "text", "text": "HELPED"}}))
}
