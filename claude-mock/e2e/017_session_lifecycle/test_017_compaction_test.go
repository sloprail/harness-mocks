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

// TestT017_07_Compaction is a compaction the way claude 2.1.282 performs one
// (a manual /compact run, the 65 real compact_boundary records, the binary):
// PreCompact, then a parentless compact_boundary APPENDED to the file — its
// logicalParentUuid the last record by default, or an unwritten one on
// request, compactMetadata {trigger, preTokens,
// preservedSegment{headUuid, anchorUuid, tailUuid}, preservedMessages{anchorUuid,
// uuids, allUuids}} with the summary as anchor — then the summary chained to
// the boundary, SessionStart:compact, and PostCompact with the summary. A
// MANUAL compaction also fires SubagentStop for its summarizer (agent_type "",
// an agent_transcript_path never written, the summary as last_assistant_message)
// between PreCompact and SessionStart, and writes the /compact command's three
// records after the summary, ahead of SessionStart:compact's attachment. The
// turn goes on after each compaction.
// sr:proves compaction-transcript-continuity/claude
// sr:proves manual-compaction/claude
// sr:proves session-start-hook/claude
func TestT017_07_Compaction(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	log := filepath.Join(dir, "payloads.log")
	h := payloadLogger(t, dir, "log.sh", log, `echo "compact-hook-said" 1>&2`)
	settings(t, dir, map[string]string{"SessionStart": h, "PreCompact": h, "PostCompact": h, "SubagentStop": h})
	sc := script(t, dir, "s",
		toolUse("b1", "Bash", `{"command":"true"}`),
		`{"type":"compact","summary":"first summary @MARK@","preserve":3,"pre_tokens":1234}`,
		toolUse("b2", "Bash", `{"command":"true"}`),
		`{"type":"compact","summary":"second summary @MARK@","trigger":"manual","logical_parent":"unwritten"}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"after both @MARK@"}]}}`,
	)
	out, code := runInDir(t, dir, nil, "--script", sc, "--session-id", "cmp-1",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)

	recs := readRecs(t, transcriptPath(t, cfg, dir, "cmp-1"))
	root, rootAt := firstRoot(recs)
	assert.Equal(t, "SessionStart:startup", root.Attachment["hookName"], "the origin stays where the session began")
	var bounds []int
	for i, r := range recs {
		if r.Subtype == "compact_boundary" {
			bounds = append(bounds, i)
		}
	}
	require.Len(t, bounds, 2, "both compactions are appended to the one file")
	assert.Less(t, rootAt, bounds[0])
	for n, bi := range bounds {
		b := recs[bi]
		assert.Nil(t, b.ParentUUID)
		if n == 0 {
			assert.Equal(t, recs[bi-1].UUID, b.LogicalParentUUID, "by default the logical parent is the segment's tail, the record right before the boundary (34 of 66 real)")
		} else {
			// "logical_parent":"unwritten": a record never written, which is
			// also allUuids' extra id (F:compact; 10 real automatic boundaries).
			assert.NotEmpty(t, b.LogicalParentUUID)
			for _, r := range recs {
				assert.NotEqual(t, b.LogicalParentUUID, r.UUID, "the manual boundary's logical parent is never written")
			}
		}
		var full map[string]any
		require.NoError(t, json.Unmarshal([]byte(b.Raw), &full))
		meta := full["compactMetadata"].(map[string]any)
		pm := meta["preservedMessages"].(map[string]any)
		seg := meta["preservedSegment"].(map[string]any)
		uuids := pm["uuids"].([]any)
		if n == 0 {
			assert.Equal(t, uuids[len(uuids)-1], b.LogicalParentUUID, "the logical parent is the preserved segment's tail")
		}
		wantKept := 3
		wantTrigger := "auto"
		if n == 1 {
			wantKept, wantTrigger = 2, "manual"
		}
		assert.Equal(t, wantTrigger, meta["trigger"])
		assert.Contains(t, meta, "preTokens")
		if n == 0 {
			assert.EqualValues(t, 1234, meta["preTokens"])
		}
		require.Len(t, uuids, wantKept, "preserve N keeps the last N records")
		for k, u := range uuids {
			assert.Equal(t, recs[bi-wantKept+k].UUID, u, "the kept records are the last ones before the boundary, in order")
		}
		if n == 0 {
			assert.Equal(t, pm["uuids"], pm["allUuids"], "nothing unwritten in the kept segment")
		} else {
			assert.Equal(t, append(append([]any{}, uuids...), b.LogicalParentUUID), pm["allUuids"],
				"allUuids is uuids plus the unwritten logical parent")
		}
		assert.Contains(t, meta, "cumulativeDroppedTokens")
		summary := recs[bi+1]
		assert.Contains(t, summary.Raw, `"isVisibleInTranscriptOnly":true`)
		assert.Contains(t, summary.Raw, `"isCompactSummary":true`)
		require.NotNil(t, summary.ParentUUID)
		assert.Equal(t, b.UUID, *summary.ParentUUID, "the summary chains to the boundary")
		assert.Equal(t, summary.UUID, pm["anchorUuid"], "the summary is the anchor")
		assert.Equal(t, summary.UUID, seg["anchorUuid"])
		assert.Equal(t, uuids[0], seg["headUuid"])
		assert.Equal(t, uuids[len(uuids)-1], seg["tailUuid"])
		next := bi + 2
		if n == 1 {
			assert.Equal(t, "<local-command-caveat>Caveat: The messages below were generated by the user while running local commands. DO NOT respond to these messages or otherwise consider them in your response unless the user explicitly asks you to.</local-command-caveat>", messageText(recs[next]))
			assert.Contains(t, recs[next].Raw, `"isMeta":true`)
			assert.Equal(t, "<command-name>/compact</command-name>\n            <command-message>compact</command-message>\n            <command-args></command-args>", messageText(recs[next+1]))
			assert.Equal(t, "<local-command-stdout>Compacted PreCompact ["+h+"] completed successfully\nPostCompact ["+h+"] completed successfully</local-command-stdout>", messageText(recs[next+2]))
			next += 3
		}
		ss := recs[next]
		assert.Equal(t, "attachment", ss.Type, "SessionStart:compact's attachment follows")
		assert.Equal(t, "SessionStart:compact", ss.Attachment["hookName"])
	}
	after := -1
	for i, r := range recs {
		if strings.Contains(r.Raw, "after both") {
			after = i
		}
	}
	assert.Greater(t, after, bounds[1], "the turn went on after both compactions")
	var events []string
	for _, p := range payloads(t, log) {
		ev := p["hook_event_name"].(string)
		switch ev {
		case "SessionStart":
			events = append(events, ev+":"+p["source"].(string))
		case "PreCompact":
			events = append(events, ev+":"+p["trigger"].(string))
			assert.Contains(t, p, "custom_instructions")
			assert.Nil(t, p["custom_instructions"])
		case "PostCompact":
			events = append(events, ev+":"+p["trigger"].(string))
			assert.Contains(t, p["compact_summary"], "summary turn-s-")
		case "SubagentStop":
			events = append(events, ev)
			assert.Equal(t, "", p["agent_type"])
			assert.Contains(t, p, "agent_type")
			assert.Regexp(t, `^a[0-9a-f]{16}$`, p["agent_id"])
			assert.Contains(t, p["last_assistant_message"], "second summary turn-s-")
			assert.Equal(t, false, p["stop_hook_active"])
			path := p["agent_transcript_path"].(string)
			assert.True(t, strings.HasSuffix(path, "/cmp-1/subagents/agent-"+p["agent_id"].(string)+".jsonl"), path)
			_, err := os.Stat(path)
			assert.True(t, os.IsNotExist(err), "the summarizer's transcript is never written")
		}
	}
	assert.Equal(t, []string{"SessionStart:startup",
		"PreCompact:auto", "SessionStart:compact", "PostCompact:auto",
		"PreCompact:manual", "SubagentStop", "SessionStart:compact", "PostCompact:manual"}, events)
	for _, r := range recs {
		if r.Type == "attachment" {
			assert.NotContains(t, r.Attachment["hookEvent"], "Compact", "Pre/PostCompact leave no attachment")
		}
	}
}

// TestT017_07c_CompactionWithoutAPreservedSegment is the second manual shape
// claude 2.1.282 left (F:compact-nohooks): no preservedSegment or
// preservedMessages, the last written record as logical parent, and token
// counts with cumulativeDroppedTokens = preTokens - postTokens, summed over the
// session's compactions.
func TestT017_07c_CompactionWithoutAPreservedSegment(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	sc := script(t, dir, "s",
		`{"type":"compact","summary":"one @MARK@","trigger":"manual","preserved_segment":false,"pre_tokens":22932,"post_tokens":2334}`,
		`{"type":"compact","summary":"two @MARK@","trigger":"manual","preserved_segment":false,"pre_tokens":5000,"post_tokens":1000}`,
	)
	out, code := runInDir(t, dir, nil, "--script", sc, "--session-id", "cmp-c",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)
	recs := readRecs(t, transcriptPath(t, cfg, dir, "cmp-c"))
	var dropped []any
	for i, r := range recs {
		if r.Subtype != "compact_boundary" {
			continue
		}
		assert.Equal(t, recs[i-1].UUID, r.LogicalParentUUID, "its logical parent is the last written record")
		var full map[string]any
		require.NoError(t, json.Unmarshal([]byte(r.Raw), &full))
		meta := full["compactMetadata"].(map[string]any)
		assert.NotContains(t, meta, "preservedSegment")
		assert.NotContains(t, meta, "preservedMessages")
		assert.Equal(t, "manual", meta["trigger"])
		dropped = append(dropped, meta["cumulativeDroppedTokens"])
	}
	assert.Equal(t, []any{float64(20598), float64(24598)}, dropped)
}

// TestT017_07d_TailEarlierThanTheLastRecord: the logical parent is the
// preserved segment's tail. In 7 real mid-file boundaries that tail is an
// EARLIER written record, 2 to 253 records before the boundary; "tail_offset"
// reproduces it.
// sr:proves compaction-transcript-continuity/claude
func TestT017_07d_TailEarlierThanTheLastRecord(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	sc := script(t, dir, "s",
		toolUse("b1", "Bash", `{"command":"true"}`),
		toolUse("b2", "Bash", `{"command":"true"}`),
		`{"type":"compact","summary":"s @MARK@","preserve":2,"tail_offset":2}`,
	)
	out, code := runInDir(t, dir, nil, "--script", sc, "--session-id", "cmp-d",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)
	recs := readRecs(t, transcriptPath(t, cfg, dir, "cmp-d"))
	var written []string
	for i, r := range recs {
		if r.Subtype != "compact_boundary" {
			if r.UUID != "" {
				written = append(written, r.UUID)
			}
			continue
		}
		var full map[string]any
		require.NoError(t, json.Unmarshal([]byte(r.Raw), &full))
		uuids := full["compactMetadata"].(map[string]any)["preservedMessages"].(map[string]any)["uuids"].([]any)
		n := len(written)
		assert.Equal(t, []any{written[n-4], written[n-3]}, uuids, "the segment ends 2 records before the boundary")
		assert.Equal(t, written[n-3], r.LogicalParentUUID, "the logical parent is the segment's tail")
		assert.NotEqual(t, recs[i-1].UUID, r.LogicalParentUUID, "not the record right before the boundary")
		return
	}
	t.Fatal("no boundary")
}

// TestT017_07e_TailOffsetBeyondTheRecordIsAnError: a tail_offset that leaves
// no written record for the segment to end on fails the run — it is a
// scenario bug, not a request to fall back silently.
func TestT017_07e_TailOffsetBeyondTheRecordIsAnError(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	sc := script(t, dir, "s", `{"type":"compact","summary":"s @MARK@","tail_offset":50}`)
	out, code := runInDir(t, dir, nil, "--script", sc, "--session-id", "cmp-e",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	assert.NotEqual(t, 0, code, out)
	assert.Contains(t, out, "tail_offset 50 leaves no record for the preserved segment to end on")
	raw, _ := os.ReadFile(transcriptPath(t, cfg, dir, "cmp-e"))
	assert.NotContains(t, string(raw), "compact_boundary", "nothing is compacted")
	sc2 := script(t, dir, "s2", `{"type":"compact","summary":"s @MARK@","tail_offset":1,"preserved_segment":false}`)
	out, code = runInDir(t, dir, nil, "--script", sc2, "--session-id", "cmp-e2",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	assert.NotEqual(t, 0, code, out)
	assert.Contains(t, out, "tail_offset needs a preserved segment")
}

// TestT017_07b_PreCompactExit2BlocksTheCompaction: PreCompact can block a
// compaction (docs, "Exit code 2 behavior per event"): nothing is written.
// sr:proves manual-compaction/claude
func TestT017_07b_PreCompactExit2BlocksTheCompaction(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	block := write(t, filepath.Join(dir, "block.sh"), "#!/bin/sh\ncat >/dev/null\necho no 1>&2\nexit 2\n", 0o755)
	settings(t, dir, map[string]string{"PreCompact": block})
	sc := script(t, dir, "s", `{"type":"compact","summary":"x @MARK@"}`, toolUse("b1", "Bash", `{"command":"true"}`))
	out, code := runInDir(t, dir, nil, "--script", sc, "--session-id", "cmp-b",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)
	raw, err := os.ReadFile(transcriptPath(t, cfg, dir, "cmp-b"))
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "compact_boundary")
	assert.NotContains(t, string(raw), "isCompactSummary")
}
