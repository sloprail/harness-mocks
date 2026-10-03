package e2e

import (
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// codexVars are the CODEX* variables of the command's output (env | grep
// ^CODEX | sort), with what differs between hosts and runs masked: the
// session's id as <SESSION_ID>, the configuration directory and the package
// root (the install directory of the launcher) as <CODEX_HOME> and <ROOT>.
func codexVars(output, sid string) []string {
	var out []string
	for _, l := range strings.Split(strings.TrimSpace(output), "\n") {
		k, v, _ := strings.Cut(l, "=")
		switch k {
		case "CODEX_HOME":
			v = "<CODEX_HOME>"
		case "CODEX_MANAGED_PACKAGE_ROOT":
			v = "<ROOT>"
		}
		if sid != "" {
			v = strings.ReplaceAll(v, sid, "<SESSION_ID>")
		}
		out = append(out, k+"="+v)
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
// mock (hooks on every event as recorded): the command sees every CODEX
// variable as Codex's did, launched bare and launched inside another session
// over decoys (this run's session id replaces the decoys', the launcher's
// CODEX_MANAGED_BY_NPM=1 replaces a decoy, a decoy CODEX_CI and CODEX_SANDBOX
// pass through).
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
			want := codexVars(lastOutput(t, recStream), sessionIDOf(t, recStream))
			require.NotEmpty(t, want)
			// All of them as recorded: the launcher's CODEX_MANAGED_BY_NPM=1
			// (over a decoy in the nested run) and CODEX_MANAGED_PACKAGE_ROOT
			// included, CODEX_SANDBOX only where inherited.
			assert.Equal(t, want, codexVars(lastOutput(t, got), sessionIDOf(t, got)))
			assert.Contains(t, want, "CODEX_MANAGED_BY_NPM=1")
			assert.Contains(t, lastOutput(t, got), "CODEX_HOME="+got.Home+"\n")
			assert.Regexp(t, `(?m)^CODEX_MANAGED_PACKAGE_ROOT=/.+$`, lastOutput(t, got), "the launcher's package root")
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

// The recorded hook processes' own CODEX* variables (hook.sh logs them on every
// event), replayed: a hook is handed the configuration directory and the
// launcher's variables, never the session's, whether launched bare or over
// decoys (a decoy CODEX_MANAGED_BY_NPM is replaced by 1, the other inherited
// decoys reach it unchanged).
// sr:proves subprocess-session-env/codex
func TestHookEnvironmentAsRecorded(t *testing.T) {
	hookEnvs := func(log []map[string]any) (out []string) {
		for _, l := range log {
			if env, ok := l["hook_env"].(map[string]any); ok {
				var kv []string
				for k, v := range env {
					kv = append(kv, k+"="+v.(string))
				}
				sort.Strings(kv)
				out = append(out, strings.Join(codexVars(strings.Join(kv, "\n"), ""), "\n"))
			}
		}
		return
	}
	for _, name := range []string{"subprocess-session-env", "nested-session-env"} {
		t.Run(name, func(t *testing.T) {
			rec := loadRecording(t, name)
			var extra []string
			for _, kv := range strings.Split(readFile(t, filepath.Join(rec.setup, "env")), "\n") {
				if kv != "" {
					extra = append(extra, kv)
				}
			}
			want := hookEnvs(jsonLines(readFile(t, filepath.Join(rec.sample, "payloads.jsonl"))))
			got := replay(t, rec, extra...)
			require.Equal(t, 0, got.Code, got.Stderr)
			require.NotEmpty(t, want)
			assert.Equal(t, want, hookEnvs(got.hookLog()))
			assert.Contains(t, want[0], "CODEX_MANAGED_BY_NPM=1")
			assert.NotContains(t, want[0], "CODEX_SESSION_ID")
		})
	}
}
