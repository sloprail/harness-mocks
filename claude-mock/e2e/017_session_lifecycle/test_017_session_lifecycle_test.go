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

// These pin the mock to how real Claude Code writes a session's record across
// its lifecycle — measured against the real transcripts on one machine, which
// is where each shape below comes from.

// TestT017_01_FreshSessionHasNoRecordAtSessionStart: a fresh session's
// transcript does not exist while SessionStart runs. All 940 real
// `SessionStart:startup` runs of a hook reading its own transcript found no
// file. When the hook prints something, its hook_success attachment is written
// after it exits, and is the file's origin; the prompt chains to it.
func TestT017_01_FreshSessionHasNoRecordAtSessionStart(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	log := filepath.Join(dir, "ss.log")
	hook := write(t, filepath.Join(dir, "ss.sh"), `#!/bin/sh
IN=$(cat)
P=$(printf '%s' "$IN" | sed -n 's/.*"transcript_path":"\([^"]*\)".*/\1/p')
if [ -e "$P" ]; then echo present >> `+log+`; else echo absent >> `+log+`; fi
echo "starting up" 1>&2
`, 0o755)
	settings(t, dir, map[string]string{"SessionStart": hook})
	sc := script(t, dir, "s")

	out, code := runInDir(t, dir, nil, "--script", sc, "--session-id", "fresh-1",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)

	logged, err := os.ReadFile(log)
	require.NoError(t, err)
	assert.Equal(t, "absent\n", string(logged), "SessionStart must find no transcript for a fresh session")

	recs := readRecs(t, transcriptPath(t, cfg, dir, "fresh-1"))
	root, i := firstRoot(recs)
	require.GreaterOrEqual(t, i, 0, "the transcript must have an origin")
	assert.Equal(t, "attachment", root.Type, "the SessionStart hook's attachment is the origin")
	assert.Equal(t, "hook_success", root.Attachment["type"])
	assert.Equal(t, "SessionStart:startup", root.Attachment["hookName"])
	assert.Equal(t, "starting up\n", root.Attachment["stderr"])
	prompt := recs[i+1]
	assert.Equal(t, "e2e-root-fresh-1", prompt.UUID)
	require.NotNil(t, prompt.ParentUUID)
	assert.Equal(t, root.UUID, *prompt.ParentUUID, "the prompt chains to the attachment")
}

// TestT017_02_SilentSessionStartLeavesThePromptAsOrigin: a hook that prints
// nothing leaves no attachment — 0 of 2,846 real PreToolUse successes are
// empty — so the prompt is the origin.
func TestT017_02_SilentSessionStartLeavesThePromptAsOrigin(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	hook := write(t, filepath.Join(dir, "ss.sh"), "#!/bin/sh\ncat >/dev/null\n", 0o755)
	settings(t, dir, map[string]string{"SessionStart": hook})
	sc := script(t, dir, "s")

	out, code := runInDir(t, dir, nil, "--script", sc, "--session-id", "fresh-2",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)
	root, _ := firstRoot(readRecs(t, transcriptPath(t, cfg, dir, "fresh-2")))
	assert.Equal(t, "e2e-root-fresh-2", root.UUID)
}

// TestT017_03_EveryHookCarriesTranscriptPath: transcript_path is one of the
// common input fields, on every event. The tool events carry tool_use_id.
func TestT017_03_EveryHookCarriesTranscriptPath(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	log := filepath.Join(dir, "payloads.log")
	h := payloadLogger(t, dir, "log.sh", log, "")
	settings(t, dir, map[string]string{
		"SessionStart": h, "UserPromptSubmit": h, "PreToolUse": h, "PostToolUse": h, "Stop": h,
	})
	sc := script(t, dir, "s", toolUse("b1", "Bash", `{"command":"true"}`))

	out, code := runInDir(t, dir, nil, "--script", sc, "--session-id", "paths-1",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)

	want := transcriptPath(t, cfg, dir, "paths-1")
	seen := map[string]bool{}
	for _, p := range payloads(t, log) {
		ev := p["hook_event_name"].(string)
		seen[ev] = true
		assert.Equal(t, want, p["transcript_path"], "%s must carry the session's transcript_path", ev)
		if ev == "PreToolUse" || ev == "PostToolUse" {
			assert.True(t, strings.HasPrefix(p["tool_use_id"].(string), "b1"), "%s must carry tool_use_id", ev)
		}
		// agent_id / agent_type are a sub-agent's; the main thread's events
		// carry neither (docs, common input fields).
		assert.NotContains(t, p, "agent_id", "%s on the main thread", ev)
		assert.NotContains(t, p, "agent_type", "%s on the main thread", ev)
	}
	for _, ev := range []string{"SessionStart", "UserPromptSubmit", "PreToolUse", "PostToolUse", "Stop"} {
		assert.True(t, seen[ev], "%s never fired", ev)
	}
}

// TestT017_04_HookAttachmentsSitWhereRealOnesDo: a PreToolUse hook that
// prints lands as hook_success AFTER its tool_use and before the result, keyed
// by the tool call's id; a non-zero, non-2 exit lands as
// hook_non_blocking_error.
func TestT017_04_HookAttachmentsSitWhereRealOnesDo(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	pre := write(t, filepath.Join(dir, "pre.sh"), "#!/bin/sh\ncat >/dev/null\necho 'pre says hi' 1>&2\n", 0o755)
	post := write(t, filepath.Join(dir, "post.sh"), "#!/bin/sh\ncat >/dev/null\necho broken 1>&2\nexit 1\n", 0o755)
	settings(t, dir, map[string]string{"PreToolUse": pre, "PostToolUse": post})
	sc := script(t, dir, "s", toolUse("b1", "Bash", `{"command":"echo ran"}`))

	out, code := runInDir(t, dir, nil, "--script", sc, "--session-id", "att-1",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)

	recs := readRecs(t, transcriptPath(t, cfg, dir, "att-1"))
	var kinds []string
	for _, r := range recs {
		switch {
		case r.Type == "attachment":
			kinds = append(kinds, r.Attachment["type"].(string)+":"+r.Attachment["hookName"].(string))
			assert.True(t, strings.HasPrefix(r.Attachment["toolUseID"].(string), "b1"), "a tool hook's attachment is keyed by the call")
		case strings.Contains(r.Raw, `"tool_use"`):
			kinds = append(kinds, "tool_use")
		case strings.Contains(r.Raw, `"tool_result"`):
			kinds = append(kinds, "tool_result")
		}
	}
	assert.Equal(t, []string{"tool_use", "hook_success:PreToolUse:Bash", "tool_result", "hook_non_blocking_error:PostToolUse:Bash"}, kinds)
	for _, r := range recs {
		if r.Type == "attachment" && r.Attachment["type"] == "hook_non_blocking_error" {
			assert.Equal(t, "Failed with non-blocking status code: broken", r.Attachment["stderr"])
		}
	}
}

// TestT017_05_SubagentPayloadsAndRecords: a sub-agent's tool calls report the
// SESSION's transcript_path with agent_id and agent_type; SubagentStop adds
// agent_transcript_path; and the sub-agent's records go to its own sidechain
// file — never into the dispatcher's (0 of 8,119 real main transcripts hold a
// sidechain record).
func TestT017_05_SubagentPayloadsAndRecords(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	log := filepath.Join(dir, "payloads.log")
	h := payloadLogger(t, dir, "log.sh", log, "")
	settings(t, dir, map[string]string{"PreToolUse": h, "SubagentStop": h})
	sub := script(t, dir, "sub", toolUse("sb1", "Bash", `{"command":"echo SUBAGENT-OUTPUT"}`))
	orch := script(t, dir, "orch", toolUse("ag1", "Agent", `{"prompt":"do it","description":"d","script":"`+sub+`"}`))

	out, code := runInDir(t, dir, nil, "--script", orch, "--session-id", "sub-1",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)

	main := transcriptPath(t, cfg, dir, "sub-1")
	var subPre, stop map[string]any
	for _, p := range payloads(t, log) {
		if p["hook_event_name"] == "PreToolUse" && strings.HasPrefix(p["tool_use_id"].(string), "sb1") {
			subPre = p
		}
		if p["hook_event_name"] == "SubagentStop" {
			stop = p
		}
	}
	require.NotNil(t, subPre, "the sub-agent's tool call fired PreToolUse")
	assert.Equal(t, main, subPre["transcript_path"], "a sub-agent's tool call reports the session's transcript")
	assert.NotEmpty(t, subPre["agent_id"])
	assert.Equal(t, "general-purpose", subPre["agent_type"])
	require.NotNil(t, stop)
	assert.Equal(t, main, stop["transcript_path"])
	side := stop["agent_transcript_path"].(string)
	assert.True(t, strings.HasSuffix(side, "/subagents/agent-"+stop["agent_id"].(string)+".jsonl"), side)

	mainRaw, err := os.ReadFile(main)
	require.NoError(t, err)
	assert.NotContains(t, string(mainRaw), "SUBAGENT-OUTPUT", "the sub-agent's records must not land in the dispatcher's file")
	assert.NotContains(t, string(mainRaw), `"isSidechain":true`)
	sideRecs := readRecs(t, side)
	found := false
	for _, r := range sideRecs {
		if strings.Contains(r.Raw, "SUBAGENT-OUTPUT") && strings.Contains(r.Raw, "tool_result") {
			found = true
			assert.True(t, r.IsSidechain)
			assert.Equal(t, stop["agent_id"], r.AgentID)
			assert.Equal(t, "sub-1", r.SessionID)
		}
	}
	assert.True(t, found, "the sub-agent's tool output is in its own file")
}

// TestT017_06_ResumeFromAnotherDirectory: a session resumed from a directory
// other than the one it began in keeps appending to its original transcript,
// while its hooks are told a transcript_path under the NEW directory's project
// folder — a file that does not exist. Measured on a real SessionStart:resume.
func TestT017_06_ResumeFromAnotherDirectory(t *testing.T) {
	root := t.TempDir()
	cfg := filepath.Join(root, "config")
	first := filepath.Join(root, "first")
	second := filepath.Join(root, "second")
	require.NoError(t, os.MkdirAll(first, 0o755))
	require.NoError(t, os.MkdirAll(second, 0o755))
	log := filepath.Join(root, "payloads.log")
	for _, d := range []string{first, second} {
		settings(t, d, map[string]string{"SessionStart": payloadLogger(t, d, "log.sh", log, "")})
	}

	out, code := runInDir(t, first, nil, "--script", script(t, first, "a"), "--session-id", "moved-1",
		"--project-dir", first, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)
	out, code = runInDir(t, second, nil, "--script", script(t, second, "b"), "--resume", "moved-1",
		"--project-dir", second, "--config-dir", cfg, "-p", "again")
	require.Equal(t, 0, code, out)

	orig := transcriptPath(t, cfg, first, "moved-1")
	reported := transcriptPath(t, cfg, second, "moved-1")
	ps := payloads(t, log)
	require.Len(t, ps, 2)
	assert.Equal(t, "resume", ps[1]["source"])
	assert.Equal(t, reported, ps[1]["transcript_path"], "the resumed session reports a path under its new directory")
	_, err := os.Stat(reported)
	assert.True(t, os.IsNotExist(err), "and nothing is written there")
	raw, err := os.ReadFile(orig)
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"again"`, "the resumed turn went to the original transcript")
}

// TestT017_07_Compaction is a compaction the way claude 2.1.282 performs one
// (a manual /compact run, the 65 real compact_boundary records, the binary):
// PreCompact, then a parentless compact_boundary APPENDED to the file — its
// logicalParentUuid the last record, compactMetadata {trigger, preTokens,
// preservedSegment{headUuid, anchorUuid, tailUuid}, preservedMessages{anchorUuid,
// uuids, allUuids}} with the summary as anchor — then the summary chained to
// the boundary, SessionStart:compact, and PostCompact with the summary. A
// MANUAL compaction also fires SubagentStop for its summarizer (agent_type "",
// an agent_transcript_path never written, the summary as last_assistant_message)
// between PreCompact and SessionStart, and writes the /compact command's three
// records after the summary, ahead of SessionStart:compact's attachment. The
// turn goes on after each compaction.
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
			assert.Equal(t, recs[bi-1].UUID, b.LogicalParentUUID, "by default the logical parent is the last record before it (55 of 66 real)")
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

// TestT017_07b_PreCompactExit2BlocksTheCompaction: PreCompact can block a
// compaction (docs, "Exit code 2 behavior per event"): nothing is written.
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

// forkRecs is a fork's records with the preamble dropped.
func forkRecs(t *testing.T, path string) []rec {
	var out []rec
	for _, r := range readRecs(t, path) {
		if r.UUID != "" {
			out = append(out, r)
		}
	}
	return out
}

// TestT017_08_ForkOfACompactedSession is the shape of all 19 real transcripts
// that open on a compact_boundary: a verbatim copy of the last boundary, the
// summary chained to it, then the records the boundary preserved, re-parented
// into one chain after the summary, then what followed the summary, and the
// fork's own turn — every record under the fork's sessionId. SessionStart
// fires with source "fork" (docs; claude 2.1.282). The original is untouched.
func TestT017_08_ForkOfACompactedSession(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	log := filepath.Join(dir, "payloads.log")
	settings(t, dir, map[string]string{"SessionStart": payloadLogger(t, dir, "log.sh", log, "")})
	sc := script(t, dir, "s",
		toolUse("b1", "Bash", `{"command":"true"}`),
		`{"type":"compact","summary":"compacted @MARK@","preserve":3}`,
	)
	out, code := runInDir(t, dir, nil, "--script", sc, "--session-id", "orig",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)
	origPath := transcriptPath(t, cfg, dir, "orig")
	before, err := os.ReadFile(origPath)
	require.NoError(t, err)
	for _, fork := range []string{"fork-a", "fork-b"} {
		out, code = runInDir(t, dir, nil, "--script", script(t, dir, fork), "--resume", "orig", "--fork-session",
			"--session-id", fork, "--project-dir", dir, "--config-dir", cfg, "-p", "continue "+fork)
		require.Equal(t, 0, code, out)
	}
	after, err := os.ReadFile(origPath)
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after), "forking leaves the original untouched")

	orig := readRecs(t, origPath)
	var boundary rec
	var preserved []string
	for _, r := range orig {
		if r.Subtype == "compact_boundary" {
			boundary = r
			var full map[string]any
			require.NoError(t, json.Unmarshal([]byte(r.Raw), &full))
			for _, u := range full["compactMetadata"].(map[string]any)["preservedMessages"].(map[string]any)["uuids"].([]any) {
				preserved = append(preserved, u.(string))
			}
		}
	}
	require.NotEmpty(t, boundary.UUID)
	require.Len(t, preserved, 3)
	for _, fork := range []string{"fork-a", "fork-b"} {
		recs := forkRecs(t, transcriptPath(t, cfg, dir, fork))
		require.GreaterOrEqual(t, len(recs), 5)
		assert.Equal(t, boundary.UUID, recs[0].UUID, "%s opens on the same boundary", fork)
		assert.Nil(t, recs[0].ParentUUID)
		assert.Equal(t, boundary.LogicalParentUUID, recs[0].LogicalParentUUID)
		assert.Contains(t, recs[1].Raw, `"isCompactSummary":true`, "then the summary")
		require.NotNil(t, recs[1].ParentUUID)
		assert.Equal(t, recs[0].UUID, *recs[1].ParentUUID)
		for k, u := range preserved {
			r := recs[2+k]
			assert.Equal(t, u, r.UUID, "then the preserved records, in order")
			require.NotNil(t, r.ParentUUID)
			assert.Equal(t, recs[1+k].UUID, *r.ParentUUID, "re-parented into one chain after the summary")
		}
		for i, r := range recs {
			assert.Equal(t, fork, r.SessionID, "record %d of %s carries the fork's session id", i, fork)
			if i > 0 {
				require.NotNil(t, r.ParentUUID, "record %d of %s", i, fork)
				assert.Equal(t, recs[i-1].UUID, *r.ParentUUID, "record %d of %s chains to the one before", i, fork)
			}
		}
		raw, _ := os.ReadFile(transcriptPath(t, cfg, dir, fork))
		assert.Contains(t, string(raw), "continue "+fork)
	}
	var sources []any
	for _, p := range payloads(t, log) {
		sources = append(sources, p["source"])
	}
	assert.Equal(t, []any{"startup", "compact", "fork", "fork"}, sources)
}

// TestT017_09_ForkOfAnUncompactedSession: a session never compacted forks
// whole — origin included, parents unchanged — under the fork's sessionId, as
// claude 2.1.282 forked one (and every 2.1.280+ fork on the machine). A fork
// of the fork works the same way.
func TestT017_09_ForkOfAnUncompactedSession(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	log := filepath.Join(dir, "payloads.log")
	settings(t, dir, map[string]string{"SessionStart": payloadLogger(t, dir, "log.sh", log, "")})
	out, code := runInDir(t, dir, nil, "--script", script(t, dir, "s", toolUse("b1", "Bash", `{"command":"true"}`)), "--session-id", "plain",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)
	origPath := transcriptPath(t, cfg, dir, "plain")
	before, _ := os.ReadFile(origPath)
	out, code = runInDir(t, dir, nil, "--script", script(t, dir, "f"), "--resume", "plain", "--fork-session",
		"--session-id", "plain-fork", "--project-dir", dir, "--config-dir", cfg, "-p", "more")
	require.Equal(t, 0, code, out)
	out, code = runInDir(t, dir, nil, "--script", script(t, dir, "g"), "--resume", "plain-fork", "--fork-session",
		"--session-id", "plain-fork-2", "--project-dir", dir, "--config-dir", cfg, "-p", "even more")
	require.Equal(t, 0, code, out)
	after, _ := os.ReadFile(origPath)
	assert.Equal(t, string(before), string(after), "forking leaves the original untouched")

	orig := forkRecs(t, origPath)
	for _, fork := range []string{"plain-fork", "plain-fork-2"} {
		recs := forkRecs(t, transcriptPath(t, cfg, dir, fork))
		require.Greater(t, len(recs), len(orig))
		for i, o := range orig {
			assert.Equal(t, o.UUID, recs[i].UUID, "%s copies record %d", fork, i)
			assert.Equal(t, o.ParentUUID, recs[i].ParentUUID, "%s keeps record %d's parent", fork, i)
			assert.Equal(t, fork, recs[i].SessionID, "%s's copy carries its own session id", fork)
		}
	}
	raw, _ := os.ReadFile(transcriptPath(t, cfg, dir, "plain-fork-2"))
	assert.Contains(t, string(raw), `"more"`, "the fork of the fork carries the first fork's turn")
	assert.Contains(t, string(raw), `"even more"`)
	var sources []any
	for _, p := range payloads(t, log) {
		sources = append(sources, p["source"])
	}
	assert.Equal(t, []any{"startup", "fork", "fork"}, sources)
}

// TestT017_10_CompactionCanNameAnUnwrittenLogicalParent: a real
// preserved-segment compaction named, as its logical parent, a record written
// to no transcript. The scenario can reproduce it.
func TestT017_10_CompactionCanNameAnUnwrittenLogicalParent(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	sc := script(t, dir, "s", `{"type":"compact","logical_parent":"never-written","summary":"x @MARK@"}`)
	out, code := runInDir(t, dir, nil, "--script", sc, "--session-id", "cmp-2",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)
	for _, r := range readRecs(t, transcriptPath(t, cfg, dir, "cmp-2")) {
		if r.Subtype == "compact_boundary" {
			assert.Equal(t, "never-written", r.LogicalParentUUID)
			var full map[string]any
			require.NoError(t, json.Unmarshal([]byte(r.Raw), &full))
			pm := full["compactMetadata"].(map[string]any)["preservedMessages"].(map[string]any)
			all := pm["allUuids"].([]any)
			assert.Equal(t, append(append([]any{}, pm["uuids"].([]any)...), "never-written"), all,
				"allUuids is uuids plus the unwritten record the boundary names (a strict superset, as in 44 of 65 real boundaries)")
			return
		}
	}
	t.Fatal("no boundary written")
}
