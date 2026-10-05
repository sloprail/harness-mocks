// Package e2e exercises the sr-agent (sloprail guardrail JUDGE) compatibility
// surface of the mock: the claude flags sr-agent passes must be ACCEPTED (cobra
// rejects undeclared flags with exit 1, which would kill the mock before it
// runs), and the CC-faithful trajectory shapes a sloprail describe/cite test
// needs must be emittable by a scenario.
//
// sr:docs https://code.claude.com/docs/en/cli-reference
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

// writeExec writes an executable file (script/hook) and returns its path.
func writeExec(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(p, []byte(body), 0o755))
	return p
}

func nonEmptyLines(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) != "" {
			out = append(out, line)
		}
	}
	return out
}

// TestT014_01_AcceptsFullSrAgentJudgeInvocation drives the mock with the EXACT
// flag shape sr-agent's BuildInvocation produces for a guardrail judge:
//
//	-p --model <m> --add-dir <dir> --allowed-tools Write \
//	   --permission-mode … --settings … --append-system-prompt … \
//	   --output-format stream-json --session-id <id> -- <prompt>
//
// The mock must NOT reject any flag (exit 1), must run the judge scenario, and
// the scenario (a Write tool_use) must produce the verdict file sr-agent's
// verifier reads. This is the whole point of the flag work: let the mock stand
// in for the judge's `claude`.
func TestT014_01_AcceptsFullSrAgentJudgeInvocation(t *testing.T) {
	dir := t.TempDir()
	verdict := filepath.Join(dir, "answer") // sr-agent names the file "answer"

	// A judge scenario: turn 1 writes the verdict via a Write tool_use; turn 2
	// (once the synthesised tool_result is in the session) emits the result frame.
	// The verdict is the {"pass":bool,…} sr-agent's `jq -e .pass` verifier reads.
	script := writeExec(t, dir, "judge.sh", `#!/bin/sh
SF="${A10N_MOCK_SESSION_FILE:-/dev/null}"
if grep -q '"tool_use_id":"tu-verdict"' "$SF" 2>/dev/null; then
  printf '%s\n' '{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"verdict written"}],"stop_reason":"end_turn"}}'
  printf '%s\n' '{"type":"result","subtype":"success","result":"done","is_error":false}'
  exit 0
fi
printf '%s\n' '{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"tu-verdict","name":"Write","input":{"file_path":"`+verdict+`","content":"{\"pass\":true,\"reasoning\":\"upholds the invariant\"}"}}]}}'
`)

	out, code := runInDir(t, dir, nil,
		"-p",
		"--model", "size-md",
		"--add-dir", dir,
		"--allowed-tools", "Write",
		"--permission-mode", "acceptEdits",
		"--settings", `{"disableAllHooks":false}`,
		"--append-system-prompt", "You are a strict judge.",
		"--output-format", "stream-json",
		"--session-id", "judge-sess-1",
		"--project-dir", dir,
		"--script", script,
		"--", "does this uphold the invariant?",
	)
	require.Equal(t, 0, code, "mock must accept the full sr-agent judge invocation; output:\n%s", out)

	// The scenario ran and wrote the verdict file the verifier reads.
	data, err := os.ReadFile(verdict)
	require.NoError(t, err, "judge scenario must have written the verdict file")
	var v struct {
		Pass      bool   `json:"pass"`
		Reasoning string `json:"reasoning"`
	}
	require.NoError(t, json.Unmarshal(data, &v), "verdict file must be JSON: %s", data)
	assert.True(t, v.Pass, "verdict.pass must be true")
	assert.NotEmpty(t, v.Reasoning)
}

// TestT014_02_AcceptsCamelCaseAndListToolFlags proves the camelCase aliases
// (--allowedTools/--disallowedTools) and the extra doc-confirmed flags are all
// accepted too — a caller reaching them via sr-agent's --claude-args pass-through
// must not crash the mock.
func TestT014_02_AcceptsCamelCaseAndListToolFlags(t *testing.T) {
	script := `#!/bin/sh
printf '%s\n' '{"type":"result","subtype":"success","result":"ok","is_error":false}'
`
	out, code := runWithScript(t, script,
		"-p",
		"--model", "opus",
		"--allowedTools", "Write,Read",
		"--disallowedTools", "Bash",
		"--input-format", "stream-json",
		"--max-budget-usd", "5",
		"--output-format", "stream-json",
		"--session-id", "camel-sess",
		"--", "go",
	)
	require.Equal(t, 0, code, "mock must accept camelCase + list tool flags; output:\n%s", out)
	assert.Contains(t, out, `"type":"result"`)
}

// TestT014_03_RejectsNonexistentPermissionPromptTool guards the grounding
// decision: --permission-prompt-tool does NOT exist in real Claude Code, so the
// mock must NOT silently accept it. (If a future change adds it, this test
// fails and forces a re-grounding against the CC docs.)
func TestT014_03_RejectsNonexistentPermissionPromptTool(t *testing.T) {
	script := `#!/bin/sh
printf '%s\n' '{"type":"result","subtype":"success","result":"ok","is_error":false}'
`
	out, code := runWithScript(t, script,
		"-p", "--model", "sonnet",
		"--permission-prompt-tool", "mcp__x__approve",
		"--session-id", "x", "--", "go",
	)
	assert.NotEqual(t, 0, code, "mock must reject a flag real claude does not have")
	assert.Contains(t, out, "unknown flag", "output should name the unknown flag; got:\n%s", out)
}

// TestT014_04_AuthoredToolResultEnvelopeAccepted proves a scenario may emit a
// user record carrying an AUTHORED tool_result (with a tool_use_id) as a
// standalone trajectory record — the real Claude Code shape for an answer
// envelope (e.g. an AskUserQuestion answer), which is delivered as a user-role
// message and is NOT synthesised from any local tool run. Before this change the
// mock's validator rejected ANY scenario-emitted user+tool_result outright, so a
// describe/cite test could not use the mock as its sole trajectory source. The
// mock must now accept it, forward it to stdout, and persist it to the transcript.
//
// The envelope stands ALONE in its turn (it references tu-ask, an id from an
// earlier assistant turn) — a tool_use always breaks the turn and is answered by
// a SYNTHESISED result, so an authored answer is a separate record, exactly as
// real CC records it.
//
// sr:docs https://code.claude.com/docs/en/sdk#stream-json-output-format
func TestT014_04_AuthoredToolResultEnvelopeAccepted(t *testing.T) {
	dir := t.TempDir()
	// The mock advances turns only via a tool_use. So turn 1 emits a real Bash
	// tool_use (mock executes it, synthesises its result, re-runs the script);
	// turn 2 — now that the Bash tool_result is in the session — emits the
	// AUTHORED answer envelope (a standalone user+tool_result referencing tu-ask,
	// which the mock did NOT synthesise) followed by the result frame that ends
	// the run. The authored envelope is a user-role record on its own, exactly as
	// real CC records an answer; the validator must accept it rather than reject
	// every scenario-authored user+tool_result.
	script := writeExec(t, dir, "ask.sh", `#!/bin/sh
SF="${A10N_MOCK_SESSION_FILE:-/dev/null}"
if grep -q '"tool_use_id":"tu-bash"' "$SF" 2>/dev/null; then
  # Turn 2: author the answer envelope, then end the run.
  printf '%s\n' '{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu-ask","content":"Yes, ship it."}]}}'
  printf '%s\n' '{"type":"result","subtype":"success","result":"answered","is_error":false}'
  exit 0
fi
# Turn 1: a real Bash tool_use so the mock advances to turn 2.
printf '%s\n' '{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"tu-bash","name":"Bash","input":{"command":"true"}}],"stop_reason":null}}'
`)

	out, code := runInDir(t, dir, nil,
		"--script", script, "--session-id", "ask-sess", "--project-dir", dir,
		"--config-dir", filepath.Join(dir, "cfg"), "-p", "go")
	require.Equal(t, 0, code, "authored tool_result envelope must be accepted; output:\n%s", out)

	// The authored tool_result reached stdout.
	assert.Contains(t, out, `"tool_use_id":"tu-ask"`, "authored tool_result must be forwarded to stdout")
	assert.Contains(t, out, "Yes, ship it.", "the authored answer content must be forwarded")

	// And it was persisted to the session transcript (so a later turn / a hook can
	// read the full trajectory including the answer envelope).
	sessionGlob := filepath.Join(dir, "cfg", "projects", "*", "ask-sess.jsonl")
	matches, err := filepath.Glob(sessionGlob)
	require.NoError(t, err)
	require.NotEmpty(t, matches, "session transcript must exist at %s", sessionGlob)
	body, err := os.ReadFile(matches[0])
	require.NoError(t, err)
	assert.Contains(t, string(body), `"tool_use_id":"tu-ask"`, "authored tool_result must be persisted to the transcript")
}

// TestT014_05_ToolResultWithoutIDRejected proves the narrow guard survives: a
// tool_result block with NO tool_use_id is malformed (real CC always references
// the answered tool_use) and must be rejected loudly.
func TestT014_05_ToolResultWithoutIDRejected(t *testing.T) {
	dir := t.TempDir()
	script := writeExec(t, dir, "bad.sh", `#!/bin/sh
printf '%s\n' '{"type":"user","message":{"role":"user","content":[{"type":"tool_result","content":"no id here"}]}}'
`)
	out, code := runInDir(t, dir, nil,
		"--script", script, "--session-id", "bad-sess", "--project-dir", dir, "-p", "go")
	assert.NotEqual(t, 0, code, "a tool_result without tool_use_id must be rejected")
	assert.Contains(t, out, "no tool_use_id", "error must explain the missing tool_use_id; got:\n%s", out)
}

// TestT014_06_SubagentMetaCarriesSpawningToolUseID proves the .meta.json sidecar
// records the SPAWNING Agent tool_use's id as toolUseId (was hardcoded "" before).
// Real Claude Code stamps this so a consumer can derive the subagent's parentPath
// (which parent tool_use spawned the sidechain). The orchestrator emits an Agent
// tool_use with id "toolu_spawn_1"; that id must land in the sidecar.
//
// sr:docs https://code.claude.com/docs/en/agent-sdk/sessions
// sr:proves subagent-transcripts/claude
func TestT014_06_SubagentMetaCarriesSpawningToolUseID(t *testing.T) {
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, "cfg")

	sub := writeExec(t, dir, "sub.sh", `#!/bin/sh
printf '%s\n' '{"type":"result","subtype":"success","result":"sub done","is_error":false}'
`)
	orch := writeExec(t, dir, "orch.sh", `#!/bin/sh
if [ -f "$A10N_MOCK_SESSION_FILE" ] && grep -q 'agentId:' "$A10N_MOCK_SESSION_FILE"; then
  printf '%s\n' '{"type":"result","subtype":"success","result":"orchestrated","is_error":false}'
  exit 0
fi
printf '%s\n' '{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"toolu_spawn_1","name":"Agent","input":{"description":"judge slice","prompt":"review","subagent_type":"general-purpose","script":"`+sub+`"}}]}}'
`)

	out, code := runInDir(t, dir, nil,
		"--script", orch, "--session-id", "meta-sess", "--project-dir", dir,
		"--config-dir", cfgDir, "-p", "go")
	require.Equal(t, 0, code, "mock should exit 0; output:\n%s", out)

	// Locate the subagent .meta.json sidecar and assert its toolUseId.
	metas, err := filepath.Glob(filepath.Join(cfgDir, "projects", "*", "meta-sess", "subagents", "agent-*.meta.json"))
	require.NoError(t, err)
	require.NotEmpty(t, metas, "a subagent .meta.json sidecar must be written")
	metaData, err := os.ReadFile(metas[0])
	require.NoError(t, err)
	var meta map[string]any
	require.NoError(t, json.Unmarshal(metaData, &meta))
	assert.Equal(t, "toolu_spawn_1", meta["toolUseId"],
		"meta.json toolUseId must be the spawning Agent tool_use id, not empty")
	assert.Equal(t, "general-purpose", meta["agentType"])
}

// TestT014_07_JudgeVerdictFileDrivesReAskLoopShape is a lightweight proof that the
// mock can, in a SINGLE run, execute several tool calls including a Bash read of
// the prompt file and a Write of the verdict — the shape a real judge scenario
// uses (read the thing under review, decide, write {"pass":…}). It confirms the
// mock is a sufficient trajectory source for the judge e2e without any hand-
// crafted jsonl: everything here comes from the mock executing the scenario.
// sr:proves file-tools/claude
func TestT014_07_JudgeReadsThenWritesVerdict(t *testing.T) {
	dir := t.TempDir()
	underReview := writeExec(t, dir, "under_review.txt", "the code does X and Y\n")
	verdict := filepath.Join(dir, "answer")

	script := writeExec(t, dir, "judge.sh", `#!/bin/sh
SF="${A10N_MOCK_SESSION_FILE:-/dev/null}"
if grep -q '"tool_use_id":"tu-write"' "$SF" 2>/dev/null; then
  printf '%s\n' '{"type":"result","subtype":"success","result":"judged","is_error":false}'
  exit 0
fi
if grep -q '"tool_use_id":"tu-read"' "$SF" 2>/dev/null; then
  printf '%s\n' '{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"tu-write","name":"Write","input":{"file_path":"`+verdict+`","content":"{\"pass\":true,\"reasoning\":\"saw X and Y\"}"}}]}}'
  exit 0
fi
printf '%s\n' '{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"tu-read","name":"Read","input":{"file_path":"`+underReview+`"}}]}}'
`)

	out, code := runInDir(t, dir, nil,
		"-p", "--model", "size-md", "--add-dir", dir, "--allowed-tools", "Write",
		"--output-format", "stream-json", "--session-id", "read-write-sess",
		"--project-dir", dir, "--script", script, "--", "judge the file")
	require.Equal(t, 0, code, "mock should exit 0; output:\n%s", out)

	// Both tool calls executed and streamed (Read result carries the file body).
	assert.Contains(t, out, "the code does X and Y", "the Read tool_result must carry the reviewed file's contents")
	lines := nonEmptyLines(out)
	assert.GreaterOrEqual(t, len(lines), 4, "expected read tool_use, read result, write tool_use, write result, and a final frame")

	data, err := os.ReadFile(verdict)
	require.NoError(t, err, "verdict file must be written")
	assert.Contains(t, string(data), `"pass":true`)
}
