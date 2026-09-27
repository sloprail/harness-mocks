package e2e

import (
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

// TestT017_07_CompactionAppendsABoundary: compaction is written into the file
// the session is writing — a parentless compact_boundary naming the last record
// as its logical parent, the summary chained to it — and fires SessionStart
// with source compact.
func TestT017_07_CompactionAppendsABoundary(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	log := filepath.Join(dir, "payloads.log")
	settings(t, dir, map[string]string{"SessionStart": payloadLogger(t, dir, "log.sh", log, "")})
	sc := script(t, dir, "s",
		toolUse("b1", "Bash", `{"command":"true"}`),
		`{"type":"compact","summary":"compacted @MARK@"}`,
	)
	out, code := runInDir(t, dir, nil, "--script", sc, "--session-id", "cmp-1",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)

	recs := readRecs(t, transcriptPath(t, cfg, dir, "cmp-1"))
	root, _ := firstRoot(recs)
	assert.Equal(t, "e2e-root-cmp-1", root.UUID, "the origin stays where the session began")
	var boundary rec
	bi := -1
	for i, r := range recs {
		if r.Subtype == "compact_boundary" {
			boundary, bi = r, i
		}
	}
	require.GreaterOrEqual(t, bi, 1, "a compact_boundary is appended part-way down")
	assert.Nil(t, boundary.ParentUUID)
	assert.Equal(t, recs[bi-1].UUID, boundary.LogicalParentUUID, "its logical parent is the last record before it")
	require.NotNil(t, recs[bi+1].ParentUUID)
	assert.Equal(t, boundary.UUID, *recs[bi+1].ParentUUID, "the summary chains to the boundary")
	sources := []any{}
	for _, p := range payloads(t, log) {
		sources = append(sources, p["source"])
	}
	assert.Equal(t, []any{"startup", "compact"}, sources)
}

// TestT017_08_ForkOfACompactedSessionOpensOnTheBoundary is the fork shape real
// resumes of a compacted session left: every fork opens on a verbatim copy of
// the same boundary record, carries the preserved record the boundary names,
// and its own sessionId.
func TestT017_08_ForkOfACompactedSessionOpensOnTheBoundary(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	sc := script(t, dir, "s",
		toolUse("b1", "Bash", `{"command":"true"}`),
		`{"type":"compact","summary":"compacted @MARK@"}`,
	)
	out, code := runInDir(t, dir, nil, "--script", sc, "--session-id", "orig",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)
	for _, fork := range []string{"fork-a", "fork-b"} {
		out, code = runInDir(t, dir, nil, "--script", script(t, dir, fork), "--resume", "orig", "--fork-session",
			"--session-id", fork, "--project-dir", dir, "--config-dir", cfg, "-p", "continue "+fork)
		require.Equal(t, 0, code, out)
	}

	var boundaryUUID, lp string
	for _, r := range readRecs(t, transcriptPath(t, cfg, dir, "orig")) {
		if r.Subtype == "compact_boundary" {
			boundaryUUID, lp = r.UUID, r.LogicalParentUUID
		}
	}
	require.NotEmpty(t, boundaryUUID)
	for _, fork := range []string{"fork-a", "fork-b"} {
		recs := readRecs(t, transcriptPath(t, cfg, dir, fork))
		root, i := firstRoot(recs)
		assert.Equal(t, boundaryUUID, root.UUID, "%s opens on the same boundary", fork)
		assert.Equal(t, lp, root.LogicalParentUUID)
		assert.Equal(t, fork, root.SessionID, "a fork's records carry its own session id")
		copied := false
		for _, r := range recs[i+1:] {
			if r.UUID == lp {
				copied = true
			}
		}
		assert.True(t, copied, "%s carries a copy of the boundary's logical parent after it", fork)
		raw, _ := os.ReadFile(transcriptPath(t, cfg, dir, fork))
		assert.Contains(t, string(raw), "continue "+fork)
	}
}

// TestT017_09_ForkOfAnUncompactedSessionSharesItsOrigin is the other real fork
// shape: the whole history, origin included.
func TestT017_09_ForkOfAnUncompactedSessionSharesItsOrigin(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	out, code := runInDir(t, dir, nil, "--script", script(t, dir, "s"), "--session-id", "plain",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)
	out, code = runInDir(t, dir, nil, "--script", script(t, dir, "f"), "--resume", "plain", "--fork-session",
		"--session-id", "plain-fork", "--project-dir", dir, "--config-dir", cfg, "-p", "more")
	require.Equal(t, 0, code, out)
	a, _ := firstRoot(readRecs(t, transcriptPath(t, cfg, dir, "plain")))
	b, _ := firstRoot(readRecs(t, transcriptPath(t, cfg, dir, "plain-fork")))
	assert.Equal(t, a.UUID, b.UUID)
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
			return
		}
	}
	t.Fatal("no boundary written")
}

// TestT017_11_BackgroundBash: the receipt, the notification, and TaskOutput.
func TestT017_11_BackgroundBash(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	// TaskOutput needs the task id, which only the receipt says: the scenario
	// reads it from the transcript.
	sc := write(t, filepath.Join(dir, "s.sh"), `#!/bin/sh
F="$A10N_MOCK_SESSION_FILE"
if ! grep -q 'bg1' "$F"; then
  echo '{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"bg1","name":"Bash","input":{"command":"echo BG-OUTPUT-4411","description":"make output","run_in_background":true}}]}}'
  exit 0
fi
if ! grep -q 'to1' "$F"; then
  ID=$(grep -o 'background with ID: [a-z0-9]*' "$F" | head -1 | sed 's/.*: //')
  echo '{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"to1","name":"TaskOutput","input":{"task_id":"'"$ID"'"}}]}}'
  exit 0
fi
echo '{"type":"assistant","message":{"role":"assistant","stop_reason":"end_turn","content":[{"type":"text","text":"done"}]}}'
echo '{"type":"result","subtype":"success","result":"done"}'
`, 0o755)
	out, code := runInDir(t, dir, nil, "--script", sc, "--session-id", "bg-1",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)

	recs := readRecs(t, transcriptPath(t, cfg, dir, "bg-1"))
	var receipt, output, notification rec
	for _, r := range recs {
		switch {
		case strings.Contains(r.Raw, `"tool_use_id":"bg1"`):
			receipt = r
		case strings.Contains(r.Raw, `"tool_use_id":"to1"`):
			output = r
		case strings.Contains(r.Raw, "<task-notification>"):
			notification = r
		}
	}
	require.NotEmpty(t, receipt.UUID)
	assert.Contains(t, receipt.Raw, "Command running in background with ID: ")
	id, _ := receipt.ToolUseResult["backgroundTaskId"].(string)
	require.NotEmpty(t, id, "the receipt's record carries toolUseResult.backgroundTaskId")
	require.NotEmpty(t, output.UUID)
	assert.Contains(t, output.Raw, "BG-OUTPUT-4411", "TaskOutput returns the command's output")
	require.NotEmpty(t, notification.UUID, "a finished task is announced with a <task-notification> turn")
	assert.Contains(t, notification.Raw, "<task-id>"+id+"</task-id>")
	assert.Contains(t, notification.Raw, "<tool-use-id>bg1</tool-use-id>")
	assert.Contains(t, notification.Raw, `"kind":"task-notification"`)
}

// TestT017_12_BackgroundAgent: the async receipt with its agentId, and the
// notification carrying the agent's reply.
func TestT017_12_BackgroundAgent(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	sub := write(t, filepath.Join(dir, "sub.sh"), `#!/bin/sh
echo '{"type":"assistant","message":{"role":"assistant","stop_reason":"end_turn","content":[{"type":"text","text":"AGENT-REPLY-7702"}]}}'
echo '{"type":"result","subtype":"success","result":"AGENT-REPLY-7702"}'
`, 0o755)
	sc := script(t, dir, "s", toolUse("ag1", "Agent", `{"prompt":"go","description":"bg agent","script":"`+sub+`","run_in_background":true}`))
	out, code := runInDir(t, dir, nil, "--script", sc, "--session-id", "bga-1",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)

	raw, err := os.ReadFile(transcriptPath(t, cfg, dir, "bga-1"))
	require.NoError(t, err)
	assert.Contains(t, string(raw), "Async agent launched successfully.")
	assert.Contains(t, string(raw), `"status":"async_launched"`)
	assert.Contains(t, string(raw), "<task-notification>")
	assert.Contains(t, string(raw), "AGENT-REPLY-7702")
}
