package e2e

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestT017_73_AgentResultCountsTheSubagentsTools: a foreground sub-agent that
// used tools hands back toolStats counting them (Read, search, Bash, edits with
// the lines they add and remove, anything else as other) and totalToolUseCount
// of all its calls, as the recorded fgsub-tool-stats run (a Write, an Edit, two
// ToolSearch, three Bash, a Read) shows; the fields are the recorded ones.
// sr:proves foreground-subagent-result/claude
func TestT017_73_AgentResultCountsTheSubagentsTools(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	log := filepath.Join(dir, "payloads.log")
	settings(t, dir, map[string]string{"PostToolUse": payloadLogger(t, dir, "log.sh", log, "")})
	notes := filepath.Join(dir, "notes.txt")
	sub := callThenReply(t, dir, "sub", "FINISHED",
		toolUse("w", "Write", `{"file_path":"`+notes+`","content":"alpha\nbeta\ngamma"}`),
		toolUse("e", "Edit", `{"file_path":"`+notes+`","old_string":"beta","new_string":"delta"}`),
		toolUse("s1", "ToolSearch", `{"query":"select:Grep","max_results":1}`),
		toolUse("s2", "ToolSearch", `{"query":"select:Glob","max_results":1}`),
		toolUse("b1", "Bash", `{"command":"grep delta `+notes+`"}`),
		toolUse("b2", "Bash", `{"command":"ls `+dir+`/*.txt"}`),
		toolUse("r", "Read", `{"file_path":"`+notes+`"}`),
		toolUse("b3", "Bash", `{"command":"echo ok"}`),
	)
	orch := script(t, dir, "orch", toolUse("ag1", "Agent", `{"prompt":"go","description":"tools","script":"`+sub+`"}`))
	out, code := runInDir(t, dir, nil, "--script", orch, "--session-id", "stat-1",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)

	got := agentPosts(payloads(t, log))[0]["tool_response"].(map[string]any)
	want := recordedAgentPost(t, "fgsub-tool-stats", 0)["tool_response"].(map[string]any)
	assert.Equal(t, want["toolStats"], got["toolStats"])
	assert.Equal(t, want["totalToolUseCount"], got["totalToolUseCount"])
	assert.Equal(t, keysOf(want), keysOf(got))
	assert.Contains(t, recordedAgentResultText(t, "fgsub-tool-stats"), "tool_uses: 8\n")
	block, _ := toolResultOf(t, readRecs(t, transcriptPath(t, cfg, dir, "stat-1")), "ag1turn-orch-a")
	assert.Contains(t, block["content"].([]any)[0].(map[string]any)["text"], "\ntool_uses: 8\n")
}

// TestT017_74_AKeptWorktreeIsNamedInTheResult: an isolated sub-agent that
// leaves a file behind keeps its worktree, and the result names it: worktreePath
// and worktreeBranch (agent-<id> under .claude/worktrees, worktree-agent-<id>)
// beside toolStats, in the fields of the recorded isolated-worktree run, and the
// hand-back's trailer gains a worktreePath line.
// sr:proves foreground-subagent-result/claude
func TestT017_74_AKeptWorktreeIsNamedInTheResult(t *testing.T) {
	dir := t.TempDir()
	gitIn(t, dir, "init", "-q")
	gitIn(t, dir, "commit", "-q", "--allow-empty", "-m", "init")
	cfg := filepath.Join(dir, "config")
	log := filepath.Join(dir, "payloads.log")
	settings(t, dir, map[string]string{"PostToolUse": payloadLogger(t, dir, "log.sh", log, "")})
	sub := callThenReply(t, dir, "sub", "done",
		toolUse("b1", "Bash", `{"command":"pwd"}`),
		toolUse("b2", "Bash", `{"command":"git branch --show-current"}`),
		toolUse("b3", "Bash", `{"command":"touch left-behind.txt"}`),
	)
	orch := script(t, dir, "orch", toolUse("ag1", "Agent", `{"prompt":"go","description":"iso","isolation":"worktree","script":"`+sub+`"}`))
	out, code := runInDir(t, dir, nil, "--script", orch, "--session-id", "kept-1",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)

	got := agentPosts(payloads(t, log))[0]["tool_response"].(map[string]any)
	want := recordedAgentPost(t, "isolated-worktree", 0)["tool_response"].(map[string]any)
	assert.Equal(t, keysOf(want), keysOf(got))
	assert.Equal(t, want["toolStats"], got["toolStats"])
	id := got["agentId"].(string)
	assert.True(t, strings.HasSuffix(got["worktreePath"].(string), "/.claude/worktrees/agent-"+id), got["worktreePath"])
	assert.Equal(t, "worktree-agent-"+id, got["worktreeBranch"])
	assert.True(t, strings.HasSuffix(want["worktreePath"].(string), "/.claude/worktrees/agent-"+want["agentId"].(string)))
	assert.Equal(t, "worktree-agent-"+want["agentId"].(string), want["worktreeBranch"])
	block, _ := toolResultOf(t, readRecs(t, transcriptPath(t, cfg, dir, "kept-1")), "ag1turn-orch-a")
	text := block["content"].([]any)[0].(map[string]any)["text"]
	assert.Contains(t, text, "\nworktreePath: "+got["worktreePath"].(string)+"\nworktreeBranch: worktree-agent-"+id+"\n<usage>")
	// the recorded hand-back names the branch the same way (runs/isolated-worktree)
	wantID := want["agentId"].(string)
	assert.Contains(t, recordedAgentResultText(t, "isolated-worktree"), "\nworktreePath: "+want["worktreePath"].(string)+"\nworktreeBranch: worktree-agent-"+wantID+"\n<usage>")
}
