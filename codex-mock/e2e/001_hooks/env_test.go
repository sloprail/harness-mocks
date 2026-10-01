package e2e

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sessionVars are the variables of the command's output that name the harness
// and the session, with the session's id as <SESSION_ID>.
func sessionVars(output, sid string) []string {
	var out []string
	for _, l := range strings.Split(strings.TrimSpace(output), "\n") {
		switch strings.SplitN(l, "=", 2)[0] {
		case "CODEX_CI", "CODEX_SESSION_ID", "CODEX_THREAD_ID", "CODEX_VERSION":
			out = append(out, strings.ReplaceAll(l, sid, "<SESSION_ID>"))
		}
	}
	return out
}

func lastOutput(t *testing.T, r result) string {
	t.Helper()
	var out string
	for _, e := range r.stream() {
		if item, _ := e["item"].(map[string]any); e["type"] == "item.completed" && item["type"] == "command_execution" {
			out, _ = item["aggregated_output"].(string)
		}
	}
	return out
}

func sessionIDOf(t *testing.T, r result) string {
	t.Helper()
	for _, e := range r.stream() {
		if e["type"] == "thread.started" {
			return e["thread_id"].(string)
		}
	}
	require.Fail(t, "no thread.started in the stream")
	return ""
}

// The recorded runs of `env | grep ^CODEX` as a tool command, replayed on the
// mock (hooks on every event as recorded): the command sees the harness and
// the session as Codex's did, launched bare and launched inside another
// session over decoys (this run's session id replaces the decoys', a decoy
// CODEX_CI passes through).
// sr:proves subprocess-session-env/codex
func TestToolCommandSeesTheSessionAsRecorded(t *testing.T) {
	for _, name := range []string{"subprocess-session-env", "nested-session-env"} {
		t.Run(name, func(t *testing.T) {
			rec := loadRecording(t, name)
			var extra []string
			for _, kv := range strings.Split(readFile(t, filepath.Join(rec.setup, "env")), "\n") {
				if kv != "" {
					extra = append(extra, kv)
				}
			}
			got := replay(t, rec, extra...)
			require.Equal(t, 0, got.Code, got.Stderr)
			recStream := result{Stdout: readFile(t, filepath.Join(rec.sample, "stream.jsonl"))}
			want := sessionVars(lastOutput(t, recStream), sessionIDOf(t, recStream))
			require.NotEmpty(t, want)
			assert.Equal(t, want, sessionVars(lastOutput(t, got), sessionIDOf(t, got)))
		})
	}
}

// A hook command is handed no session variable (recorded): the session is in
// its payload, and an inherited CODEX_THREAD_ID reaches it unchanged.
// sr:proves subprocess-session-env/codex
func TestHookCommandGetsNoSessionVariable(t *testing.T) {
	hook := `in=$(cat); printf '%s\n' "$in" >>"$HOOK_LOG"; jq -cn '{hook_env: (env | with_entries(select(.key | startswith("CODEX"))))}' >>"$HOOK_LOG"`
	for name, env := range map[string][]string{"bare": nil, "nested": {"CODEX_THREAD_ID=decoy-outer", "CODEX_SESSION_ID=decoy-session"}} {
		t.Run(name, func(t *testing.T) {
			r := execMock(t, scenario{
				HooksJSON: hooksJSON("sh hook.sh", "SessionStart", "PreToolUse"),
				Files:     map[string]string{"hook.sh": hook},
				Script:    callThenResult, Prompt: "go", Env: append(withCalls(t, "true"), env...),
			})
			require.Equal(t, 0, r.Code, r.Stderr)
			var sawPayload bool
			for _, l := range r.hookLog() {
				hookEnv, ok := l["hook_env"].(map[string]any)
				if !ok {
					sawPayload = sawPayload || l["session_id"] != nil
					continue
				}
				assert.Equal(t, r.Home, hookEnv["CODEX_HOME"])
				if name == "bare" {
					assert.NotContains(t, hookEnv, "CODEX_THREAD_ID", "a hook is not given the session id")
					assert.NotContains(t, hookEnv, "CODEX_SESSION_ID")
				} else {
					assert.Equal(t, "decoy-outer", hookEnv["CODEX_THREAD_ID"], "an inherited value reaches a hook unchanged")
					assert.Equal(t, "decoy-session", hookEnv["CODEX_SESSION_ID"])
				}
			}
			assert.True(t, sawPayload, "the payload carries the session id")
		})
	}
}
