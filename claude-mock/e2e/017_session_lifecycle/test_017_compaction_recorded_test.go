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

func recordedFile(t *testing.T, glob string) string {
	t.Helper()
	m, err := filepath.Glob(glob)
	require.NoError(t, err)
	require.Len(t, m, 1, glob)
	return m[0]
}

// boundaryOf is the compact_boundary record of a transcript file, as a map.
func boundariesOf(t *testing.T, path string) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, r := range readRecs(t, path) {
		if r.Subtype == "compact_boundary" {
			var m map[string]any
			require.NoError(t, json.Unmarshal([]byte(r.Raw), &m))
			out = append(out, m)
		}
	}
	return out
}

// TestT017_67_BoundaryWithoutAPreservedSegmentAsRecorded holds the mock's
// second boundary shape to the compact-nohooks run: the boundary is the
// transcript's chain break (a null parentUuid) pointing back through
// logicalParentUuid at the last written record; its compactMetadata is exactly
// trigger, preTokens, durationMs, postTokens and cumulativeDroppedTokens, with
// no preservedSegment and no preservedMessages; and the summary record is
// chained to the boundary.
// sr:proves compaction-transcript-continuity/claude
func TestT017_67_BoundaryWithoutAPreservedSegmentAsRecorded(t *testing.T) {
	realPath := recordedFile(t, "../../snapshots/runs/compact-nohooks/samples/*/transcript/*.jsonl")
	real := boundariesOf(t, realPath)
	require.Len(t, real, 1)
	realRecs := readRecs(t, realPath)
	realMeta := real[0]["compactMetadata"].(map[string]any)
	assert.Nil(t, real[0]["parentUuid"])
	for _, k := range []string{"preservedSegment", "preservedMessages"} {
		assert.NotContains(t, realMeta, k, "recorded: no preserved tail in this shape")
	}
	var recordedSummaryParent string
	for i, r := range realRecs {
		if r.Subtype == "compact_boundary" {
			last := ""
			for _, before := range realRecs[:i] {
				if before.UUID != "" {
					last = before.UUID
				}
			}
			assert.Equal(t, last, r.LogicalParentUUID, "recorded: the logical parent is the last written record")
			assert.Equal(t, r.UUID, *realRecs[i+1].ParentUUID, "recorded: the summary chains to the boundary")
			recordedSummaryParent = *realRecs[i+1].ParentUUID
		}
	}
	require.NotEmpty(t, recordedSummaryParent)

	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	sc := script(t, dir, "s",
		toolUse("b1", "Bash", `{"command":"true"}`),
		`{"type":"compact","summary":"sum @MARK@","trigger":"manual","preserved_segment":false,"pre_tokens":21111,"post_tokens":2014}`,
	)
	out, code := runInDir(t, dir, nil, "--script", sc, "--session-id", "cmp-nh",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)
	path := transcriptPath(t, cfg, dir, "cmp-nh")
	recs := readRecs(t, path)
	got := boundariesOf(t, path)
	require.Len(t, got, 1)
	assert.Nil(t, got[0]["parentUuid"], "the boundary breaks the parent chain")
	meta := got[0]["compactMetadata"].(map[string]any)
	assert.Equal(t, keysOf(realMeta), keysOf(meta), "compactMetadata carries the recorded fields and no more")
	assert.EqualValues(t, 21111, meta["preTokens"])
	assert.EqualValues(t, 2014, meta["postTokens"])
	assert.EqualValues(t, 19097, meta["cumulativeDroppedTokens"], "recorded: preTokens - postTokens (21111 - 2014)")
	for i, r := range recs {
		if r.Subtype == "compact_boundary" {
			assert.Equal(t, recs[i-1].UUID, r.LogicalParentUUID, "the logical parent is the last written record")
			require.NotNil(t, recs[i+1].ParentUUID)
			assert.Equal(t, r.UUID, *recs[i+1].ParentUUID, "the summary chains to the boundary")
		}
	}
}

// TestT017_68_CompactionHookFramesAsRecorded: in the compact run the stream
// carries hook_started/hook_response frames for the SessionStart hooks only
// (SessionStart:resume before the compaction, SessionStart:compact between the
// "compacting" status and the status that ends it); PreCompact, PostCompact and
// SubagentStop, though configured and fired, stream no such frames. The
// mock's stream follows.
// sr:proves compaction-transcript-continuity/claude
func TestT017_68_CompactionHookFramesAsRecorded(t *testing.T) {
	type frame struct{ typ, sub, name, status string }
	read := func(raw string) []frame {
		var out []frame
		for _, l := range strings.Split(raw, "\n") {
			var f map[string]any
			if json.Unmarshal([]byte(l), &f) != nil || f == nil || f["type"] != "system" {
				continue
			}
			sub, _ := f["subtype"].(string)
			name, _ := f["hook_name"].(string)
			st, _ := f["status"].(string)
			if f["compact_result"] != nil {
				st = "compact_result:" + f["compact_result"].(string)
			}
			out = append(out, frame{"system", sub, name, st})
		}
		return out
	}
	// the hook frames inside the compaction window: after "compacting", before its end.
	window := func(fs []frame) []string {
		var names []string
		in := false
		for _, f := range fs {
			switch {
			case f.sub == "status" && f.status == "compacting":
				in = true
			case f.sub == "status" && f.status == "compact_result:success":
				in = false
			case in && (f.sub == "hook_started" || f.sub == "hook_response"):
				names = append(names, f.sub+" "+f.name)
			}
		}
		return names
	}
	raw, err := os.ReadFile(recordedFile(t, "../../snapshots/runs/compact/samples/*/stream.jsonl"))
	require.NoError(t, err)
	recorded := window(read(string(raw)))
	assert.Equal(t, []string{"hook_started SessionStart:compact", "hook_response SessionStart:compact"}, recorded)
	for _, f := range read(string(raw)) {
		assert.NotContains(t, f.name, "PreCompact")
		assert.NotContains(t, f.name, "PostCompact")
		assert.NotContains(t, f.name, "SubagentStop")
	}

	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	log := filepath.Join(dir, "payloads.log")
	h := payloadLogger(t, dir, "log.sh", log, "")
	settings(t, dir, map[string]string{"SessionStart": h, "PreCompact": h, "PostCompact": h})
	sc := script(t, dir, "s", toolUse("b1", "Bash", `{"command":"true"}`),
		`{"type":"compact","summary":"sum @MARK@","trigger":"manual"}`)
	out, code := runInDir(t, dir, nil, "--script", sc, "--session-id", "cmp-hf",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)
	got := read(out)
	assert.Equal(t, recorded, window(got), "the SessionStart:compact frames sit inside the compaction window, as recorded")
	for _, f := range got {
		assert.NotContains(t, f.name, "PreCompact", "PreCompact streams no hook frames, as recorded")
		assert.NotContains(t, f.name, "PostCompact", "PostCompact streams no hook frames, as recorded")
	}
	var fired []any
	for _, p := range payloads(t, log) {
		fired = append(fired, p["hook_event_name"])
	}
	assert.Contains(t, fired, "PreCompact", "the hooks fired though they stream nothing")
	assert.Contains(t, fired, "PostCompact")
}
