package e2e

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// contextKinds are the [type, hookName, content] of a transcript's hook_additional_context
// attachments, each checked to follow the hook_success of the same hook.
func contextKinds(t *testing.T, recs []rec) (out [][3]string) {
	t.Helper()
	for i, r := range recs {
		if r.Attachment["type"] != "hook_additional_context" {
			continue
		}
		require.Greater(t, i, 0)
		assert.Equal(t, "hook_success", recs[i-1].Attachment["type"], "it follows the hook's hook_success")
		assert.Equal(t, r.Attachment["hookEvent"], recs[i-1].Attachment["hookEvent"], "of the same event")
		content := r.Attachment["content"].([]any)
		require.Len(t, content, 1)
		out = append(out, [3]string{r.AgentID, r.Attachment["hookName"].(string), content[0].(string)})
	}
	return out
}

// TestT017_98_SubagentPostToolUseContext: a PostToolUse hook's context for a tool a
// sub-agent called is added to the sub-agent's own transcript (its records marked
// isSidechain, with the sub-agent's agentId), after the hook_success for that
// call and under that call's name; the main thread's transcript holds the context
// for the Agent call alone (recording subagent-post-ctx, which the test reads).
// sr:proves hook-additional-context/claude
func TestT017_98_SubagentPostToolUseContext(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	h := write(t, filepath.Join(dir, "post.sh"), `#!/bin/sh
if grep -q '"agent_type"' >/dev/null; then WHO=sub; else WHO=main; fi
printf '{"hookSpecificOutput":{"hookEventName":"PostToolUse","additionalContext":"POST-CTX-%s"}}' "$WHO"
`, 0o755)
	settings(t, dir, map[string]string{"PostToolUse": h})
	sub := script(t, dir, "sub", toolUse("sb1", "Bash", `{"command":"true"}`))
	orch := script(t, dir, "orch", toolUse("ag1", "Agent", `{"prompt":"do it","description":"d","script":"`+sub+`"}`))
	out, code := runInDir(t, dir, nil, "--script", orch, "--session-id", "sc-1",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)

	main := transcriptPath(t, cfg, dir, "sc-1")
	got := contextKinds(t, readRecs(t, main))
	assert.Equal(t, [][3]string{{"", "PostToolUse:Agent", "POST-CTX-main"}}, got, "the main thread holds the Agent call's context only")
	files, err := filepath.Glob(filepath.Join(strings.TrimSuffix(main, ".jsonl"), "subagents", "agent-*.jsonl"))
	require.NoError(t, err)
	require.Len(t, files, 1)
	recs := readRecs(t, files[0])
	agentID := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(files[0]), "agent-"), ".jsonl")
	assert.Equal(t, [][3]string{{agentID, "PostToolUse:Bash", "POST-CTX-sub"}}, contextKinds(t, recs))
	// as recorded: the same two places, the same hook names, the sub-agent's own records marked
	runs := "../../snapshots/runs/subagent-post-ctx/samples/*/transcript/"
	recMain := readRecs(t, recordedFile(t, runs+"*.jsonl"))
	recSub := readRecs(t, recordedFile(t, runs+"*/subagents/agent-*.jsonl"))
	names := func(kinds [][3]string) (out []string) {
		for _, k := range kinds {
			out = append(out, k[1])
		}
		return
	}
	assert.Equal(t, names(contextKinds(t, recMain)), names(got))
	assert.Equal(t, names(contextKinds(t, recSub)), names(contextKinds(t, recs)))
	for _, r := range recSub {
		if r.Attachment["type"] == "hook_additional_context" {
			assert.True(t, r.IsSidechain)
		}
	}
	for _, r := range recs {
		if r.Attachment["type"] == "hook_additional_context" {
			assert.True(t, r.IsSidechain, "the sub-agent's own record")
			assert.True(t, strings.HasPrefix(r.Attachment["toolUseID"].(string), "sb1"), "under the sub-agent's call")
		}
	}
}

// TestT017_99_SessionStartContextOnResume: a SessionStart hook's context is added again
// when a session is resumed: the transcript holds one for the first start (source
// startup) and one for the resume, in order, as the recording resume-session-start-ctx shows.
// sr:proves hook-additional-context/claude
func TestT017_99_SessionStartContextOnResume(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	h := write(t, filepath.Join(dir, "ss.sh"), `#!/bin/sh
if grep -q '"source":"resume"' >/dev/null; then SRC=resume; else SRC=startup; fi
printf '{"hookSpecificOutput":{"hookEventName":"SessionStart","additionalContext":"START-CTX-%s"}}' "$SRC"
`, 0o755)
	settings(t, dir, map[string]string{"SessionStart": h})
	for _, args := range [][]string{{"--session-id", "rs-1", "-p", "first"}, {"--resume", "rs-1", "-p", "again"}} {
		out, code := runInDir(t, dir, nil, append([]string{"--script", script(t, dir, "s"), "--project-dir", dir, "--config-dir", cfg}, args...)...)
		require.Equal(t, 0, code, out)
	}
	want := [][3]string{{"", "SessionStart", "START-CTX-startup"}, {"", "SessionStart", "START-CTX-resume"}}
	recorded := readRecs(t, recordedFile(t, "../../snapshots/runs/resume-session-start-ctx/samples/*/transcript/*.jsonl"))
	assert.Equal(t, want, contextKinds(t, recorded), "recorded")
	assert.Equal(t, want, contextKinds(t, readRecs(t, transcriptPath(t, cfg, dir, "rs-1"))))
}
