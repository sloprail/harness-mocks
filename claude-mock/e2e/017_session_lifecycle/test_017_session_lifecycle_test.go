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
// staged:proves hook-output-transcript-records/claude
// sr:proves session-transcript-file/claude
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
// staged:proves hook-output-transcript-records/claude
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
// staged:proves hook-common-payload/claude
// sr:proves session-transcript-file/claude
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
// sr:proves hook-exit-code-semantics/claude
// staged:proves hook-output-transcript-records/claude
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
// staged:proves hook-common-payload/claude
// sr:proves subagent-lifecycle-hooks/claude
// sr:proves subagent-transcripts/claude
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
// sr:proves session-resume/claude
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
