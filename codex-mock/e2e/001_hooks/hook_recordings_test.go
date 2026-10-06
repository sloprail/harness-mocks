package e2e

import (
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordedPayloads are the hook payloads a recorded run logged, in order.
func recordedPayloads(t *testing.T, rec recording) []map[string]any {
	t.Helper()
	return jsonLines(readFile(t, filepath.Join(rec.sample, "payloads.jsonl")))
}

// payloadsOf are the payloads of one event, from a hook log.
func payloadsOf(lines []map[string]any, event string) []map[string]any {
	var out []map[string]any
	for _, l := range lines {
		if l["hook_event_name"] == event {
			out = append(out, l)
		}
	}
	return out
}

// osNeutral is a payload value without the OS's own wording of an error (ls's
// message differs between systems), which the recording and the mock may differ in.
func osNeutral(v any) any {
	if s, ok := v.(string); ok && strings.Contains(s, "No such file or directory") {
		return "<ENOENT>"
	}
	return v
}

// replayAs replays a recorded run on the mock like replay, starting the
// session in dir (a subdirectory of the repository) and with the run's
// project-level hooks.json, when it has one.
func replayAs(t *testing.T, rec recording, dir string) result {
	t.Helper()
	return execMock(t, scenario{
		HooksJSON:        readFile(t, filepath.Join(rec.setup, "hooks.json")),
		ProjectHooksJSON: readFile(t, filepath.Join(rec.setup, "project-hooks.json")),
		Files:            map[string]string{"hook.sh": readFile(t, filepath.Join(rec.setup, "hook.sh"))},
		Script:           callThenResult,
		Prompt:           strings.TrimSpace(readFile(t, filepath.Join(rec.setup, "prompt.txt"))),
		Env:              withCalls(t, rec.calls...),
		Dir:              dir,
	})
}

// The recorded runs that make a call: the mock's PostToolUse payloads carry
// the very fields, with the very values, of the real ones. hook-exit-codes also
// holds a command that exits non-zero (its PostToolUse still fires, with the
// error text as the response) and one a before-tool hook refused (none fires).
// sr:proves posttooluse-payload/codex
func TestPostToolUsePayloadFieldsOfRecordedRuns(t *testing.T) {
	for _, name := range []string{"stops", "hook-exit-codes", "posttool-block", "shell-exit-status"} {
		t.Run(name, func(t *testing.T) {
			rec := loadRecording(t, name)
			recorded := recordedPayloads(t, rec)
			recPost := payloadsOf(recorded, "PostToolUse")
			require.NotEmpty(t, recPost)
			if len(payloadsOf(recorded, "PreToolUse")) == 0 { // the calls are those the payloads name (no shell-quote parsing)
				rec.calls = nil
				for _, p := range recPost {
					rec.calls = append(rec.calls, p["tool_input"].(map[string]any)["command"].(string))
				}
			}
			got := replay(t, rec)
			require.Equal(t, 0, got.Code, got.Stderr)
			gotPost := payloadsOf(got.hookLog(), "PostToolUse")
			require.Len(t, gotPost, len(recPost), "one PostToolUse per executed call")

			// what the recording shows of every payload: its fields and their types
			sid := sessionIDOf(t, got)
			for i, want := range recPost {
				p := gotPost[i]
				assert.Equal(t, keysOf(want), keysOf(p), "the fields of PostToolUse #%d", i)
				for _, k := range []string{"hook_event_name", "permission_mode", "tool_name", "tool_input", "tool_response"} {
					assert.Equal(t, osNeutral(want[k]), osNeutral(p[k]), "%s of PostToolUse #%d (the OS's wording of an error aside)", k, i)
				}
				assert.Equal(t, "Bash", p["tool_name"])
				assert.Equal(t, sid, p["session_id"])
				assert.NotEmpty(t, p["turn_id"])
				assert.NotEmpty(t, p["tool_use_id"])
				assert.NotContains(t, p, "duration_ms")
			}

			if name == "hook-exit-codes" {
				// `echo one` was refused by PreToolUse (exit 2): no PostToolUse for it. The failed
				// `false` (non-zero exit, prints nothing) has one, whose response is the command's empty output.
				for _, post := range [][]map[string]any{recPost, gotPost} {
					var cmds, resps []string
					for _, p := range post {
						cmds = append(cmds, p["tool_input"].(map[string]any)["command"].(string))
						resps = append(resps, p["tool_response"].(string))
					}
					assert.Equal(t, []string{"false", "echo three"}, cmds)
					assert.Equal(t, "", resps[0])
					assert.Equal(t, "three\n", resps[1])
				}
			}
		})
	}
}

// A PostToolUse hook that exits 2 does not stop the call's payload from being
// sent, or the call from having run; the agent gets the hook's stderr in place
// of the result and, as the recording shows, tries the command again, which
// sends the same payload again with a call id of its own (runs/posttool-block:
// echo POSTBLOCK twice, then echo AFTER, all in one turn).
// sr:proves posttooluse-payload/codex
func TestPostToolUseExit2PayloadIsRepeatedForTheRetry(t *testing.T) {
	rec := loadRecording(t, "posttool-block")
	var recorded []map[string]any
	for _, p := range payloadsOf(recordedPayloads(t, rec), "PostToolUse") {
		recorded = append(recorded, p)
	}
	require.Len(t, recorded, 3)
	assert.Equal(t, recorded[0]["tool_input"], recorded[1]["tool_input"])
	assert.NotEqual(t, recorded[0]["tool_use_id"], recorded[1]["tool_use_id"])
	assert.Contains(t, readFile(t, transcriptOf(t, rec)), "POST-FEEDBACK-MSG")

	got := replay(t, rec)
	require.Equal(t, 0, got.Code, got.Stderr)
	posted := payloadsOf(got.hookLog(), "PostToolUse")
	require.Len(t, posted, 3)
	for i, p := range posted {
		assert.Equal(t, recorded[i]["tool_input"], p["tool_input"], "call #%d", i)
		assert.Equal(t, recorded[i]["tool_response"], p["tool_response"], "call #%d", i)
		assert.Equal(t, recorded[i]["permission_mode"], p["permission_mode"])
	}
	assert.Equal(t, posted[0]["tool_input"], posted[1]["tool_input"])
	assert.NotEqual(t, posted[0]["tool_use_id"], posted[1]["tool_use_id"])
	assert.Len(t, map[any]bool{posted[0]["turn_id"]: true, posted[1]["turn_id"]: true, posted[2]["turn_id"]: true}, 1)
	assert.Contains(t, got.rollout(t), "POST-FEEDBACK-MSG", "the agent was not given the hook's feedback")
}

// transcriptOf is the rollout file a recorded run kept.
func transcriptOf(t *testing.T, rec recording) string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(rec.sample, "transcript", "*.jsonl"))
	require.NoError(t, err)
	require.Len(t, files, 1)
	return files[0]
}

// A prompt refused by a UserPromptSubmit hook (exit 2) never reaches the agent: no tool call, no Stop hook
// (the turn did not run), neither the prompt nor the reason in the rollout,
// and the payload the hook saw is the recorded one (prompt, permission mode).
// runs/prompt-blocked.
// sr:proves user-prompt-submit-hook/codex
func TestRefusedPromptNeverReachesTheAgent(t *testing.T) {
	for name, reason := range map[string]string{"prompt-blocked": "PROMPT-BLOCK-MSG"} {
		t.Run(name, func(t *testing.T) {
			rec := loadRecording(t, name)
			recorded := recordedPayloads(t, rec)
			want := payloadsOf(recorded, "UserPromptSubmit")
			require.Len(t, want, 1)
			assert.Empty(t, payloadsOf(recorded, "Stop"), "the recording has a Stop hook for a refused prompt")
			assert.Empty(t, payloadsOf(recorded, "PreToolUse"))
			assert.NotContains(t, readFile(t, transcriptOf(t, rec)), "SHOULDNOTRUN")

			marker := filepath.Join(t.TempDir(), "script-ran")
			got := execMock(t, scenario{
				HooksJSON: readFile(t, filepath.Join(rec.setup, "hooks.json")),
				Files:     map[string]string{"hook.sh": readFile(t, filepath.Join(rec.setup, "hook.sh"))},
				Script:    "touch " + marker + "\n" + callThenResult,
				Prompt:    strings.TrimSpace(readFile(t, filepath.Join(rec.setup, "prompt.txt"))),
				Env:       withCalls(t, rec.calls...),
			})
			require.Equal(t, 0, got.Code, got.Stderr)
			assert.NoFileExists(t, marker, "the agent ran for a refused prompt")
			assert.Equal(t, []string{"thread.started", "turn.started", "turn.completed"}, streamShape(got.stream()))
			cmds, _ := got.commands()
			assert.Empty(t, cmds)
			assert.Empty(t, payloadsOf(got.hookLog(), "Stop"), "a Stop hook fired for a turn that never ran")
			assert.Empty(t, payloadsOf(got.hookLog(), "PreToolUse"))
			assert.NotContains(t, got.rollout(t), "SHOULDNOTRUN")
			assert.NotContains(t, got.rollout(t), reason)

			posted := payloadsOf(got.hookLog(), "UserPromptSubmit")
			require.Len(t, posted, 1)
			assert.Equal(t, keysOf(want[0]), keysOf(posted[0]))
			for _, k := range []string{"hook_event_name", "permission_mode", "prompt"} {
				assert.Equal(t, want[0][k], posted[0][k], k)
			}
			assert.NotEmpty(t, posted[0]["turn_id"])
			assert.Equal(t, sessionIDOf(t, got), posted[0]["session_id"])
		})
	}
}

// A UserPromptSubmit hook adds context to the prompt as JSON
// hookSpecificOutput.additionalContext too: the agent receives it as a
// developer message of exactly that text, the prompt itself is not changed,
// and the turn runs and ends (runs/user-prompt-submit-json-context).
// sr:proves user-prompt-submit-hook/codex
func TestUserPromptSubmitJSONAdditionalContextIsAddedNotRewritten(t *testing.T) {
	rec := loadRecording(t, "user-prompt-submit-json-context")
	prompt := strings.TrimSpace(readFile(t, filepath.Join(rec.setup, "prompt.txt")))
	var recDev []string
	recDev = append(recDev, developerTexts(t, readFile(t, transcriptOf(t, rec)))...)
	require.Contains(t, recDev, "The secret word is BANANA.", "the recording: the hook's context reached the agent")
	recPrompt := payloadsOf(recordedPayloads(t, rec), "UserPromptSubmit")
	require.Len(t, recPrompt, 1)
	assert.Equal(t, prompt, recPrompt[0]["prompt"])

	got := replay(t, rec)
	require.Equal(t, 0, got.Code, got.Stderr)
	assert.Contains(t, developerTexts(t, got.rollout(t)), "The secret word is BANANA.")
	assert.NotContains(t, got.rollout(t), "hookSpecificOutput", "the JSON itself was handed on")
	posted := payloadsOf(got.hookLog(), "UserPromptSubmit")
	require.Len(t, posted, 1)
	assert.Equal(t, prompt, posted[0]["prompt"])
	assert.Equal(t, recPrompt[0]["permission_mode"], posted[0]["permission_mode"])
	assert.Len(t, payloadsOf(got.hookLog(), "Stop"), 1, "the turn ran to its end")
	assert.Equal(t, streamShape(result{Stdout: readFile(t, filepath.Join(rec.sample, "stream.jsonl"))}.stream()), streamShape(got.stream()))
}

// The matcher is not used for UserPromptSubmit: a hook whose matcher
// matches nothing still runs, and receives the prompt (runs/hook-matchers).
// sr:proves user-prompt-submit-hook/codex
func TestUserPromptSubmitIgnoresItsMatcher(t *testing.T) {
	rec := loadRecording(t, "hook-matchers")
	var recRan bool
	for _, l := range recordedPayloads(t, rec) {
		recRan = recRan || (l["ran"] == "prompt-ignored-matcher" && l["event"] == "UserPromptSubmit")
	}
	require.True(t, recRan, "the recording: a hook with matcher zzz-never ran for the prompt")

	r := execMock(t, scenario{
		HooksJSON: `{"hooks":{"UserPromptSubmit":[{"matcher":"zzz-never","hooks":[{"type":"command","command":"sh hook.sh"}]}]}}`,
		Files:     map[string]string{"hook.sh": logHook},
		Script:    callThenResult, Prompt: "the prompt", Env: withCalls(t),
	})
	posted := eventsOf(r, "UserPromptSubmit")
	require.Len(t, posted, 1)
	assert.Equal(t, "the prompt", posted[0]["prompt"])
}

// A command hook is run through the shell in the directory the session was
// started in, a subdirectory of the repository included, with the payload on
// its stdin; the form the docs recommend for repo-local hooks, a command
// substitution resolving the git root ("$(git rev-parse --show-toplevel)"/hook.sh),
// finds the script from there (runs/hook-command-subdir: started in .codex).
// sr:proves hook-command-handler/codex
func TestCommandHookRunsInASubdirectoryAndResolvesTheGitRoot(t *testing.T) {
	rec := loadRecording(t, "hook-command-subdir")
	recorded := recordedPayloads(t, rec)
	recPwd := func(lines []map[string]any) (out []string) {
		for _, l := range lines {
			if p, ok := l["hook_pwd"].(string); ok {
				out = append(out, p)
			}
		}
		return
	}
	assert.Equal(t, []string{"<RUN>/.codex", "<RUN>/.codex", "<RUN>/.codex", "<RUN>/.codex", "<RUN>/.codex"}, recPwd(recorded), "the recording: every hook ran in the subdirectory")
	for _, p := range recorded {
		if p["hook_event_name"] != nil {
			assert.Equal(t, "<RUN>/.codex", p["cwd"], p["hook_event_name"])
		}
	}

	got := replayAs(t, rec, ".codex")
	require.Equal(t, 0, got.Code, got.Stderr)
	log := got.hookLog()
	want, err := filepath.EvalSymlinks(filepath.Join(got.Repo, ".codex"))
	require.NoError(t, err)
	pwds := recPwd(log)
	require.Len(t, pwds, 5, "the hook was found through the git root from the subdirectory, for every event")
	for _, p := range pwds {
		assert.Equal(t, want, p)
	}
	var events []string
	for _, p := range log {
		if ev, ok := p["hook_event_name"].(string); ok {
			events = append(events, ev)
			cwd, err := filepath.EvalSymlinks(p["cwd"].(string))
			require.NoError(t, err)
			assert.Equal(t, want, cwd, ev+" payload cwd")
			assert.Equal(t, sessionIDOf(t, got), p["session_id"], ev)
		}
	}
	assert.Equal(t, []string{"SessionStart", "UserPromptSubmit", "PreToolUse", "PostToolUse", "Stop"}, events)
	post := payloadsOf(log, "PostToolUse")
	assert.Equal(t, map[string]any{"command": "echo SUBDIR"}, post[0]["tool_input"], "the payload reached the hook's stdin")
	stop := payloadsOf(log, "Stop")
	require.Len(t, stop, 1)
	assert.Equal(t, false, stop[0]["stop_hook_active"], "the Stop payload reached the hook's stdin")
	assert.Equal(t, "DONE", stop[0]["last_assistant_message"])
	// the agent's own command runs in the session directory too
	cmds, _ := got.commands()
	assert.Equal(t, []string{"echo SUBDIR"}, cmds)
}

// A hook group with no matcher key at all matches every event, as
// the docs say and the recording shows, whatever the tool or event subject,
// while a group beside it with a matcher that matches nothing does not run
// (runs/hook-matcher-omitted, read through hooks.json).
// sr:proves hook-matcher-filter/codex
func TestOmittedMatcherMatchesEverything(t *testing.T) {
	rec := loadRecording(t, "hook-matcher-omitted")
	ran := func(lines []map[string]any) (out []string) {
		for _, l := range lines {
			if s, ok := l["ran"].(string); ok {
				out = append(out, s)
			}
		}
		sort.Strings(out)
		return
	}
	want := []string{"post-omitted", "pre-omitted", "prompt-omitted", "start-omitted", "stop-omitted"}
	assert.Equal(t, want, ran(recordedPayloads(t, rec)), "the recording")
	assert.NotContains(t, readFile(t, filepath.Join(rec.setup, "hooks.json")), `"matcher": ""`)

	got := replay(t, rec)
	require.Equal(t, 0, got.Code, got.Stderr)
	assert.Equal(t, want, ran(got.hookLog()))
	var tools []string
	for _, l := range got.hookLog() {
		if l["event"] == "PreToolUse" || l["event"] == "PostToolUse" {
			tools = append(tools, l["tool"].(string))
		}
	}
	assert.Equal(t, []string{"Bash", "Bash"}, tools)
}
