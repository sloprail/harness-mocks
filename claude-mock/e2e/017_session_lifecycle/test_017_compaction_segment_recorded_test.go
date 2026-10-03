package e2e

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mapsOf(t *testing.T, path string, keep func(map[string]any) bool) []map[string]any {
	t.Helper()
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	var out []map[string]any
	for _, l := range strings.Split(string(raw), "\n") {
		var m map[string]any
		if json.Unmarshal([]byte(l), &m) == nil && m != nil && keep(m) {
			out = append(out, m)
		}
	}
	return out
}

// TestT017_76_BoundaryWithAPreservedSegmentAsRecorded holds the mock's manual
// compaction that keeps a tail to the compact run's transcript: the boundary
// record is a system compact_boundary with content "Conversation compacted",
// level info, isMeta false, a null parentUuid, and compactMetadata of exactly
// trigger, preTokens, durationMs, preservedSegment, preservedMessages, postTokens
// and cumulativeDroppedTokens, the segment naming head, anchor and tail and the
// messages naming anchor, uuids and allUuids; the recorded logicalParentUuid is
// a record never written, which allUuids lists after the kept ones; and the
// summary that follows is a user record flagged isCompactSummary, chained to the
// boundary, whose text, the mock's default one, opens as the real one does.
// sr:proves compaction-transcript-continuity/claude
func TestT017_76_BoundaryWithAPreservedSegmentAsRecorded(t *testing.T) {
	isBoundary := func(m map[string]any) bool { return m["subtype"] == "compact_boundary" }
	isSummary := func(m map[string]any) bool { return m["isCompactSummary"] == true }
	real := recordedFile(t, "../../snapshots/runs/compact/samples/*/transcript/*.jsonl")
	rb := mapsOf(t, real, isBoundary)
	rs := mapsOf(t, real, isSummary)
	require.Len(t, rb, 1)
	require.Len(t, rs, 1)
	rmeta := rb[0]["compactMetadata"].(map[string]any)
	rmsgs := rmeta["preservedMessages"].(map[string]any)
	assert.Equal(t, rb[0]["uuid"], rs[0]["parentUuid"], "recorded: the summary chains to the boundary")
	assert.NotContains(t, rmsgs["uuids"], rb[0]["logicalParentUuid"], "recorded: the logical parent is not a kept record")
	assert.Equal(t, rb[0]["logicalParentUuid"], rmsgs["allUuids"].([]any)[len(rmsgs["allUuids"].([]any))-1], "recorded: allUuids ends with it")
	const summaryOpening = "This session is being continued from a previous conversation"
	var rtext struct{ Content string }
	rawMsg, _ := json.Marshal(rs[0]["message"])
	require.NoError(t, json.Unmarshal(rawMsg, &rtext))
	require.True(t, strings.HasPrefix(rtext.Content, summaryOpening), rtext.Content)

	dir := t.TempDir()
	cfg := dir + "/config"
	sc := script(t, dir, "s",
		toolUse("b1", "Bash", `{"command":"true"}`),
		`{"type":"compact","summary":"","id":"@MARK@","trigger":"manual","logical_parent":"never-written","pre_tokens":23568,"post_tokens":2400}`)
	out, code := runInDir(t, dir, nil, "--script", sc, "--session-id", "seg-1",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)
	path := transcriptPath(t, cfg, dir, "seg-1")
	gb := mapsOf(t, path, isBoundary)
	gs := mapsOf(t, path, isSummary)
	require.Len(t, gb, 1)
	require.Len(t, gs, 1)

	assert.Nil(t, gb[0]["parentUuid"])
	assert.Equal(t, "system", gb[0]["type"])
	for _, k := range []string{"content", "level", "isMeta"} {
		assert.Equal(t, rb[0][k], gb[0][k], k)
	}
	gmeta := gb[0]["compactMetadata"].(map[string]any)
	assert.Equal(t, keysOf(rmeta), keysOf(gmeta), "compactMetadata carries the recorded fields and no more")
	assert.Equal(t, keysOf(rmeta["preservedSegment"].(map[string]any)), keysOf(gmeta["preservedSegment"].(map[string]any)))
	gmsgs := gmeta["preservedMessages"].(map[string]any)
	assert.Equal(t, keysOf(rmsgs), keysOf(gmsgs))
	assert.EqualValues(t, 23568-2400, gmeta["cumulativeDroppedTokens"])
	all := gmsgs["allUuids"].([]any)
	assert.Equal(t, "never-written", gb[0]["logicalParentUuid"])
	assert.Equal(t, "never-written", all[len(all)-1], "allUuids ends with the unwritten logical parent")
	assert.Equal(t, append(append([]any{}, gmsgs["uuids"].([]any)...), "never-written"), all)
	seg := gmeta["preservedSegment"].(map[string]any)
	uuids := gmsgs["uuids"].([]any)
	assert.Equal(t, uuids[0], seg["headUuid"])
	assert.Equal(t, uuids[len(uuids)-1], seg["tailUuid"])

	assert.Equal(t, gb[0]["uuid"], gs[0]["parentUuid"], "the summary chains to the boundary")
	for _, k := range []string{"isCompactSummary", "isVisibleInTranscriptOnly", "type"} {
		assert.Equal(t, rs[0][k], gs[0][k], k)
	}
	var gtext struct{ Content string }
	gotMsg, _ := json.Marshal(gs[0]["message"])
	require.NoError(t, json.Unmarshal(gotMsg, &gtext))
	assert.True(t, strings.HasPrefix(gtext.Content, summaryOpening), "the summary opens as the real one does: %s", gtext.Content)
}
