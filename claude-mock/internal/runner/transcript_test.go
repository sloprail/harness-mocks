package runner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sloprail/harness-mocks/claude-mock/internal/hooks"
)

// recordsOf runs fn against a fresh transcript and returns what it wrote.
func recordsOf(t *testing.T, fn func(tr *transcript)) []map[string]any {
	t.Helper()
	path := filepath.Join(t.TempDir(), "s.jsonl")
	tr, err := openTranscript(path, path, recordStamp{SessionID: "s", Cwd: "/w", IsSidechain: true, AgentID: "a1"})
	require.NoError(t, err)
	fn(tr)
	tr.Close()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	require.NoError(t, err)
	var out []map[string]any
	for _, l := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var m map[string]any
		require.NoError(t, json.Unmarshal([]byte(l), &m))
		out = append(out, m)
	}
	return out
}

func att(m map[string]any) map[string]any {
	a, _ := m["attachment"].(map[string]any)
	return a
}

// staged:proves hook-output-transcript-records/claude
func TestRecordHookRuns_SilentSuccessLeavesNothing(t *testing.T) {
	recs := recordsOf(t, func(tr *transcript) {
		tr.recordHookRuns(hooks.Input{HookEventName: hooks.EventPreToolUse, ToolName: "Bash", ToolUseID: "toolu_1"},
			[]hooks.HandlerRun{{Command: "h"}})
	})
	assert.Empty(t, recs)
}

// staged:proves hook-output-transcript-records/claude
func TestRecordHookRuns_SuccessWithOutput(t *testing.T) {
	recs := recordsOf(t, func(tr *transcript) {
		tr.recordHookRuns(hooks.Input{HookEventName: hooks.EventPreToolUse, ToolName: "Bash", ToolUseID: "toolu_1"},
			[]hooks.HandlerRun{{Command: "h", Stderr: "note\n", DurationMs: 7}})
	})
	require.Len(t, recs, 1)
	a := att(recs[0])
	assert.Equal(t, map[string]any{
		"type": "hook_success", "hookName": "PreToolUse:Bash", "toolUseID": "toolu_1", "hookEvent": "PreToolUse",
		"content": "", "stdout": "", "stderr": "note\n", "exitCode": float64(0), "command": "h", "durationMs": float64(7),
	}, a)
	assert.Equal(t, true, recs[0]["isSidechain"])
	assert.Equal(t, "a1", recs[0]["agentId"])
}

// staged:proves hook-output-transcript-records/claude
func TestRecordHookRuns_PlainStdoutIsTheContent(t *testing.T) {
	recs := recordsOf(t, func(tr *transcript) {
		tr.recordHookRuns(hooks.Input{HookEventName: hooks.EventUserPromptSubmit}, []hooks.HandlerRun{{Command: "h", Stdout: "hello\n"}})
	})
	require.Len(t, recs, 1)
	assert.Equal(t, "hello", att(recs[0])["content"])
	assert.Equal(t, "UserPromptSubmit", att(recs[0])["hookName"])
}

// staged:proves hook-additional-context/claude
func TestRecordHookRuns_AdditionalContextPair(t *testing.T) {
	out := hooks.Output{HookSpecificOutput: &hooks.HookSpecificOutput{AdditionalContext: "CTX"}}
	recs := recordsOf(t, func(tr *transcript) {
		tr.recordHookRuns(hooks.Input{HookEventName: hooks.EventPostToolUse, ToolName: "Bash", ToolUseID: "toolu_1"},
			[]hooks.HandlerRun{{Command: "h", Stdout: `{"hookSpecificOutput":{"additionalContext":"CTX"}}`, Output: out}})
		tr.recordHookRuns(hooks.Input{HookEventName: hooks.EventSessionStart, Source: "startup"},
			[]hooks.HandlerRun{{Command: "h", Stdout: `{"hookSpecificOutput":{"additionalContext":"CTX"}}`, Output: out}})
	})
	require.Len(t, recs, 4)
	assert.Equal(t, "hook_success", att(recs[0])["type"])
	assert.Equal(t, "", att(recs[0])["content"], "JSON stdout is not content")
	assert.Equal(t, map[string]any{"type": "hook_additional_context", "content": []any{"CTX"}, "hookName": "PostToolUse:Bash", "toolUseID": "toolu_1", "hookEvent": "PostToolUse"}, att(recs[1]))
	assert.Equal(t, "SessionStart:startup", att(recs[2])["hookName"])
	assert.Equal(t, map[string]any{"type": "hook_additional_context", "content": []any{"CTX"}, "hookName": "SessionStart", "toolUseID": "SessionStart", "hookEvent": "SessionStart"}, att(recs[3]))
}

// staged:proves hook-exit-code-semantics/claude
// staged:proves hook-output-transcript-records/claude
func TestRecordHookRuns_NonBlockingError(t *testing.T) {
	recs := recordsOf(t, func(tr *transcript) {
		tr.recordHookRuns(hooks.Input{HookEventName: hooks.EventPostToolUse, ToolName: "Bash", ToolUseID: "toolu_1"},
			[]hooks.HandlerRun{{Command: "h", ExitCode: 1, Stderr: "  broken \n"}, {Command: "g", ExitCode: 3}})
	})
	require.Len(t, recs, 2)
	assert.Equal(t, "Failed with non-blocking status code: broken", att(recs[0])["stderr"])
	assert.Equal(t, "Failed with non-blocking status code: No stderr output", att(recs[1])["stderr"])
	assert.Equal(t, float64(3), att(recs[1])["exitCode"])
	assert.Contains(t, att(recs[1]), "durationMs")
}

// staged:proves hook-exit-code-semantics/claude
// staged:proves hook-output-transcript-records/claude
func TestRecordHookRuns_Exit2PerEvent(t *testing.T) {
	blocked := []hooks.HandlerRun{{Command: "h", ExitCode: 2, Blocked: true, Stderr: "no\n"}}
	for _, tc := range []struct {
		in   hooks.Input
		want func(t *testing.T, recs []map[string]any)
	}{
		{hooks.Input{HookEventName: hooks.EventSessionStart, Source: "startup"}, func(t *testing.T, recs []map[string]any) {
			require.Len(t, recs, 1)
			assert.Equal(t, map[string]any{"type": "hook_non_blocking_error", "hookName": "SessionStart:startup", "toolUseID": att(recs[0])["toolUseID"],
				"hookEvent": "SessionStart", "stderr": "[h]: no\n", "stdout": "", "exitCode": float64(2), "command": "h"}, att(recs[0]))
		}},
		{hooks.Input{HookEventName: hooks.EventSubagentStart, AgentType: "worker"}, func(t *testing.T, recs []map[string]any) {
			require.Len(t, recs, 1)
			assert.Equal(t, "SubagentStart:worker", att(recs[0])["hookName"])
			assert.Equal(t, "hook_non_blocking_error", att(recs[0])["type"])
		}},
		{hooks.Input{HookEventName: hooks.EventPreToolUse, ToolName: "Bash", ToolUseID: "toolu_1"}, func(t *testing.T, recs []map[string]any) {
			assert.Empty(t, recs, "the refusal is the tool_result")
		}},
		{hooks.Input{HookEventName: hooks.EventPostToolUse, ToolName: "Bash", ToolUseID: "toolu_1"}, func(t *testing.T, recs []map[string]any) {
			require.Len(t, recs, 1)
			assert.Equal(t, map[string]any{"blockingError": "[h]: no\n", "command": "h"}, att(recs[0])["blockingError"])
		}},
		{hooks.Input{HookEventName: hooks.EventSubagentStop}, func(t *testing.T, recs []map[string]any) {
			require.Len(t, recs, 1, "the feedback turn only")
			assert.Equal(t, true, recs[0]["isMeta"])
			assert.Equal(t, "Stop hook feedback:\n[h]: no\n", recs[0]["message"].(map[string]any)["content"])
		}},
		{hooks.Input{HookEventName: hooks.EventStop}, func(t *testing.T, recs []map[string]any) {
			require.Len(t, recs, 2, "the feedback turn, then the summary")
			assert.Equal(t, "stop_hook_summary", recs[1]["subtype"])
			assert.Equal(t, []any{"[h]: no\n"}, recs[1]["hookErrors"])
			assert.Equal(t, []any{map[string]any{"command": "h"}}, recs[1]["hookInfos"])
		}},
		{hooks.Input{HookEventName: hooks.EventUserPromptSubmit}, func(t *testing.T, recs []map[string]any) {
			assert.Empty(t, recs, "no evidence, nothing written")
		}},
	} {
		t.Run(string(tc.in.HookEventName), func(t *testing.T) {
			tc.want(t, recordsOf(t, func(tr *transcript) { tr.recordHookRuns(tc.in, blocked) }))
		})
	}
}

// staged:proves hook-output-transcript-records/claude
func TestRecordHookRuns_EmptyStderrOnExit2(t *testing.T) {
	recs := recordsOf(t, func(tr *transcript) {
		tr.recordHookRuns(hooks.Input{HookEventName: hooks.EventSubagentStop}, []hooks.HandlerRun{{Command: "h", ExitCode: 2, Blocked: true}})
	})
	require.Len(t, recs, 1)
	assert.Equal(t, "Stop hook feedback:\n[h]: No stderr output", recs[0]["message"].(map[string]any)["content"])
}

// staged:proves hook-output-transcript-records/claude
func TestRecordHookRuns_JSONStopBlock(t *testing.T) {
	recs := recordsOf(t, func(tr *transcript) {
		tr.recordHookRuns(hooks.Input{HookEventName: hooks.EventStop}, []hooks.HandlerRun{
			{Command: "h", Stdout: `{"decision":"block"}`, Output: hooks.Output{Decision: "block"}},
			{Command: "g", DurationMs: 4},
		})
	})
	require.Len(t, recs, 3)
	assert.Equal(t, "Stop hook feedback:\nBlocked by hook", recs[0]["message"].(map[string]any)["content"], "an empty reason reads Blocked by hook")
	a := att(recs[1])
	assert.Equal(t, "hook_blocking_error", a["type"])
	assert.Equal(t, map[string]any{"blockingError": "Blocked by hook", "command": "h"}, a["blockingError"])
	s := recs[2]
	assert.Equal(t, "stop_hook_summary", s["subtype"])
	assert.Equal(t, a["toolUseID"], s["toolUseID"])
	assert.Equal(t, float64(2), s["hookCount"])
	assert.Equal(t, []any{map[string]any{"command": "h"}, map[string]any{"command": "g", "durationMs": float64(4)}}, s["hookInfos"])
	assert.Equal(t, true, s["hasOutput"])
	assert.Equal(t, false, s["preventedContinuation"])
	assert.Equal(t, "suggestion", s["level"])
}

// staged:proves hook-output-transcript-records/claude
func TestRecordHookRuns_UnrecordedEvents(t *testing.T) {
	for _, ev := range []hooks.EventName{hooks.EventSessionEnd, hooks.EventPreCompact, hooks.EventPostCompact, hooks.EventWorktreeCreate, hooks.EventWorktreeRemove} {
		recs := recordsOf(t, func(tr *transcript) {
			tr.recordHookRuns(hooks.Input{HookEventName: ev}, []hooks.HandlerRun{{Command: "h", Stdout: "x", Stderr: "y", ExitCode: 1}})
		})
		assert.Empty(t, recs, "%s leaves no record", ev)
	}
}

func TestRecordHookRuns_PreToolUseDenyLeavesNothing(t *testing.T) {
	deny := hooks.Output{HookSpecificOutput: &hooks.HookSpecificOutput{PermissionDecision: "deny", PermissionDecisionReason: "r"}}
	recs := recordsOf(t, func(tr *transcript) {
		tr.recordHookRuns(hooks.Input{HookEventName: hooks.EventPreToolUse, ToolName: "Bash", ToolUseID: "toolu_1"},
			[]hooks.HandlerRun{{Command: "h", Stdout: `{"hookSpecificOutput":{"permissionDecision":"deny"}}`, Output: deny}})
	})
	assert.Empty(t, recs)
	assert.Equal(t, "r", denyReason(deny))
	assert.Equal(t, "Blocked by hook", denyReason(hooks.Output{Decision: "block"}))
}

// staged:proves transcript-record-envelope/claude
func TestStampRecord_MainAndSidechain(t *testing.T) {
	main := stampRecord([]byte(`{"type":"user"}`), recordStamp{SessionID: "s", Cwd: "/w", GitBranch: "b"})
	var m map[string]any
	require.NoError(t, json.Unmarshal(main, &m))
	assert.Equal(t, false, m["isSidechain"])
	assert.Equal(t, "external", m["userType"])
	assert.Equal(t, "sdk-cli", m["entrypoint"])
	assert.Equal(t, "2.1.282", m["version"])
	assert.Equal(t, "b", m["gitBranch"])
	assert.NotContains(t, m, "agentId")
	side := stampRecord([]byte(`{"type":"user","isSidechain":true}`), recordStamp{SessionID: "s", IsSidechain: true, AgentID: "a"})
	require.NoError(t, json.Unmarshal(side, &m))
	assert.Equal(t, "a", m["agentId"])
	noBranch := stampRecord([]byte(`{"type":"user"}`), recordStamp{SessionID: "s"})
	var n map[string]any
	require.NoError(t, json.Unmarshal(noBranch, &n))
	assert.NotContains(t, n, "gitBranch", "outside a repository gitBranch is left off")
}

func TestLastUUIDs(t *testing.T) {
	recs := recordsOf(t, func(tr *transcript) {
		for _, u := range []string{"u1", "u2", "u3"} {
			tr.persistMap(map[string]any{"type": "user", "uuid": u})
		}
		assert.Equal(t, []string{"u2", "u3"}, tr.lastUUIDs(2))
		assert.Equal(t, []string{"u1", "u2", "u3"}, tr.lastUUIDs(9))
		assert.Equal(t, []string{}, tr.lastUUIDs(0))
	})
	require.Len(t, recs, 3)
}
