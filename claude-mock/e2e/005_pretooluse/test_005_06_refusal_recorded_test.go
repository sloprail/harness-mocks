package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// refusalScenario runs the recorded stops scenario's tool calls: Bash "echo
// DENYME" (refused by a JSON deny), "echo EXIT2ME" (refused by exit 2) and
// "echo FINE" (allowed), each after the previous one's result, then a final
// reply. One hook serves PreToolUse, PostToolUse and PostToolUseFailure and
// logs what fired after a tool. It returns the run's output, the transcript
// and the post-tool log.
func refusalScenario(t *testing.T) (out, transcript, postLog string, hook string) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "cfg")
	logFile := filepath.Join(dir, "post.log")
	hook = filepath.Join(dir, "hook.sh")
	require.NoError(t, os.WriteFile(hook, []byte(`#!/bin/sh
IN=$(cat)
case "$IN" in
  *'"hook_event_name":"PreToolUse"'*)
    case "$IN" in
      *DENYME*) echo '{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"DENY-REASON"}}' ;;
      *EXIT2ME*) echo "PRE-BLOCK-MSG" >&2; exit 2 ;;
    esac ;;
  *) printf '%s\n' "$IN" >> "`+logFile+`" ;;
esac
exit 0
`), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".claude"), 0o755))
	entry := `[{"matcher":"*","hooks":[{"type":"command","command":"` + hook + `"}]}]`
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".claude", "settings.json"), []byte(
		`{"hooks":{"PreToolUse":`+entry+`,"PostToolUse":`+entry+`,"PostToolUseFailure":`+entry+`}}`), 0o644))
	call := func(id, cmd string) string {
		return `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"` + id + `","name":"Bash","input":{"command":"` + cmd + `"}}]}}`
	}
	script := filepath.Join(dir, "s.sh")
	require.NoError(t, os.WriteFile(script, []byte(`#!/bin/sh
SF="$A10N_MOCK_SESSION_FILE"
if ! grep -q '"tool_use_id":"t1"' "$SF"; then printf '%s\n' '`+call("t1", "echo DENYME")+`'; exit 0; fi
if ! grep -q '"tool_use_id":"t2"' "$SF"; then printf '%s\n' '`+call("t2", "echo EXIT2ME")+`'; exit 0; fi
if ! grep -q '"tool_use_id":"t3"' "$SF"; then printf '%s\n' '`+call("t3", "echo FINE")+`'; exit 0; fi
printf '%s\n' '{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"DONE"}]}}'
printf '%s\n' '{"type":"result","subtype":"success","result":"DONE","is_error":false}'
`), 0o755))
	out, code := runInDir(t, dir, nil, "--script", script, "--session-id", "s-ref", "--project-dir", dir, "--config-dir", cfg, "-p", "go")
	require.Equal(t, 0, code, "the turn goes on after each refusal; output:\n%s", out)
	files, _ := filepath.Glob(filepath.Join(cfg, "projects", "*", "s-ref.jsonl"))
	require.Len(t, files, 1)
	raw, err := os.ReadFile(files[0])
	require.NoError(t, err)
	logged, _ := os.ReadFile(logFile)
	return out, string(raw), string(logged), hook
}

// toolResults is each tool_result in the transcript by tool_use_id.
func toolResults(t *testing.T, transcript string) map[string]map[string]any {
	got := map[string]map[string]any{}
	for _, l := range strings.Split(transcript, "\n") {
		var rec struct {
			Type    string `json:"type"`
			Message struct {
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal([]byte(l), &rec) != nil || rec.Type != "user" {
			continue
		}
		var blocks []map[string]any
		if json.Unmarshal(rec.Message.Content, &blocks) != nil {
			continue
		}
		for _, b := range blocks {
			if b["type"] == "tool_result" {
				got[b["tool_use_id"].(string)] = b
			}
		}
	}
	return got
}

// Both forms of refusal reach the agent as the call's error result, worded as
// recorded ("PreToolUse:Bash hook error: DENY-REASON" for the JSON deny, the
// quoted command and stderr for exit 2); the refused calls never run, the
// allowed one does, and the turn goes on to its end (recorded:
// snapshots/runs/stops).
// sr:docs https://code.claude.com/docs/en/hooks#pretooluse-decision-control
// sr:proves pretooluse-refusal/claude
func TestT005_06_RefusalsAreTheCallsErrorResults(t *testing.T) {
	out, transcript, _, hook := refusalScenario(t)
	res := toolResults(t, transcript)
	require.Len(t, res, 3, "transcript:\n%s", transcript)
	assert.Equal(t, "PreToolUse:Bash hook error: DENY-REASON", res["t1"]["content"])
	assert.Equal(t, true, res["t1"]["is_error"])
	assert.Equal(t, "PreToolUse:Bash hook error: ["+hook+"]: PRE-BLOCK-MSG\n", res["t2"]["content"])
	assert.Equal(t, true, res["t2"]["is_error"])
	assert.Equal(t, "FINE", res["t3"]["content"])
	assert.NotEqual(t, true, res["t3"]["is_error"])
	assert.Contains(t, out, `"result":"DONE"`, "the turn reaches its end")
}

// A refused call fires neither the success nor the failure hook; the allowed
// call fires only the success hook (recorded: snapshots/runs/stops, where
// PostToolUse fired for echo FINE alone).
// sr:docs https://code.claude.com/docs/en/hooks#posttoolusefailure
// sr:proves tool-failure-hook/claude
// sr:proves pretooluse-refusal/claude
func TestT005_06_RefusedCallsFireNoPostToolHook(t *testing.T) {
	_, _, postLog, _ := refusalScenario(t)
	lines := strings.Split(strings.TrimSpace(postLog), "\n")
	require.Len(t, lines, 1, "post-tool log:\n%s", postLog)
	var p map[string]any
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &p))
	assert.Equal(t, "PostToolUse", p["hook_event_name"])
	assert.Equal(t, "echo FINE", p["tool_input"].(map[string]any)["command"])
	assert.NotContains(t, postLog, "DENYME")
	assert.NotContains(t, postLog, "EXIT2ME")
	assert.NotContains(t, postLog, "PostToolUseFailure")
}

// preToolRun runs one Bash call ("touch <marker>") under the given
// PreToolUse handlers (each a JSON body the hook prints) and returns whether
// the call ran and its tool_result.
func preToolRun(t *testing.T, bodies ...string) (ran bool, result map[string]any) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "cfg")
	marker := filepath.Join(dir, "ran")
	var hooks []string
	for i, body := range bodies {
		p := filepath.Join(dir, fmt.Sprintf("h%d.sh", i))
		require.NoError(t, os.WriteFile(p, []byte("#!/bin/sh\ncat >/dev/null\necho '"+body+"'\n"), 0o755))
		hooks = append(hooks, `{"type":"command","command":"`+p+`"}`)
	}
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".claude"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".claude", "settings.json"), []byte(
		`{"hooks":{"PreToolUse":[{"matcher":"*","hooks":[`+strings.Join(hooks, ",")+`]}]}}`), 0o644))
	script := filepath.Join(dir, "s.sh")
	require.NoError(t, os.WriteFile(script, []byte(`#!/bin/sh
if ! grep -q '"tool_use_id":"t1"' "$A10N_MOCK_SESSION_FILE"; then
  printf '%s\n' '{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"touch `+marker+`"}}]}}'
  exit 0
fi
printf '%s\n' '{"type":"result","subtype":"success","result":"DONE","is_error":false}'
`), 0o755))
	out, code := runInDir(t, dir, nil, "--script", script, "--session-id", "s-pt", "--project-dir", dir, "--config-dir", cfg, "-p", "go")
	require.Equal(t, 0, code, "output:\n%s", out)
	files, _ := filepath.Glob(filepath.Join(cfg, "projects", "*", "s-pt.jsonl"))
	require.Len(t, files, 1)
	raw, _ := os.ReadFile(files[0])
	_, err := os.Stat(marker)
	return err == nil, toolResults(t, string(raw))["t1"]
}

func decision(d string) string {
	return `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"` + d + `","permissionDecisionReason":"` + strings.ToUpper(d) + `-REASON"}}`
}

// The deprecated top-level decision maps as documented: "block" is a deny,
// refusing the call with its reason; "approve" is an allow, and the call runs.
// A block stands against another hook's approve, in either order.
// sr:docs https://code.claude.com/docs/en/hooks#pretooluse-decision-control
// sr:proves pretooluse-refusal/claude
func TestT005_06_DeprecatedDecisionMapsToDenyAndAllow(t *testing.T) {
	block := `{"decision":"block","reason":"OLD-BLOCK-REASON"}`
	approve := `{"decision":"approve","reason":"OLD-APPROVE-REASON"}`
	ran, res := preToolRun(t, block)
	assert.False(t, ran, "block refuses the call")
	assert.Equal(t, "PreToolUse:Bash hook error: OLD-BLOCK-REASON", res["content"])
	assert.Equal(t, true, res["is_error"])
	ran, res = preToolRun(t, approve)
	assert.True(t, ran, "approve lets the call run")
	assert.NotEqual(t, true, res["is_error"])
	for _, order := range [][]string{{block, approve}, {approve, block}} {
		ran, res = preToolRun(t, order...)
		assert.False(t, ran, "a block stands against an approve")
		assert.Equal(t, "PreToolUse:Bash hook error: OLD-BLOCK-REASON", res["content"])
	}
}

// A deny wins over ask and defer as well as allow, whichever hook runs first
// (deny > defer > ask > allow); without a deny, ask and defer refuse nothing
// in the mock's non-interactive run (a declared deviation).
// sr:docs https://code.claude.com/docs/en/hooks#pretooluse-decision-control
// sr:proves pretooluse-refusal/claude
func TestT005_06_DenyWinsOverAskAndDefer(t *testing.T) {
	for _, other := range []string{"allow", "ask", "defer"} {
		for _, order := range [][]string{{decision("deny"), decision(other)}, {decision(other), decision("deny")}} {
			ran, res := preToolRun(t, order...)
			assert.False(t, ran, "deny beats %s", other)
			assert.Equal(t, "PreToolUse:Bash hook error: DENY-REASON", res["content"], "deny beats %s", other)
		}
		ran, _ := preToolRun(t, decision(other))
		assert.True(t, ran, "%s alone refuses nothing", other)
	}
}

// When several PreToolUse hooks decide one call, a deny wins over an allow in
// whichever order they are configured: the call is refused with the deny's
// reason and does not run.
// sr:docs https://code.claude.com/docs/en/hooks#pretooluse-decision-control
// sr:proves pretooluse-refusal/claude
func TestT005_06_DenyWinsOverAllow(t *testing.T) {
	for _, order := range []string{"deny-first", "allow-first"} {
		t.Run(order, func(t *testing.T) {
			dir := t.TempDir()
			cfg := filepath.Join(dir, "cfg")
			marker := filepath.Join(dir, "ran")
			decide := func(name, decision string) string {
				p := filepath.Join(dir, name)
				require.NoError(t, os.WriteFile(p, []byte(`#!/bin/sh
cat >/dev/null
echo '{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"`+decision+`","permissionDecisionReason":"`+strings.ToUpper(decision)+`-REASON"}}'
`), 0o755))
				return `{"type":"command","command":"` + p + `"}`
			}
			deny, allow := decide("deny.sh", "deny"), decide("allow.sh", "allow")
			hooks := deny + "," + allow
			if order == "allow-first" {
				hooks = allow + "," + deny
			}
			require.NoError(t, os.MkdirAll(filepath.Join(dir, ".claude"), 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(dir, ".claude", "settings.json"), []byte(
				`{"hooks":{"PreToolUse":[{"matcher":"*","hooks":[`+hooks+`]}]}}`), 0o644))
			script := filepath.Join(dir, "s.sh")
			require.NoError(t, os.WriteFile(script, []byte(`#!/bin/sh
if ! grep -q '"tool_use_id":"t1"' "$A10N_MOCK_SESSION_FILE"; then
  printf '%s\n' '{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"touch `+marker+`"}}]}}'
  exit 0
fi
printf '%s\n' '{"type":"result","subtype":"success","result":"DONE","is_error":false}'
`), 0o755))
			out, code := runInDir(t, dir, nil, "--script", script, "--session-id", "s-dw", "--project-dir", dir, "--config-dir", cfg, "-p", "go")
			require.Equal(t, 0, code, "output:\n%s", out)
			_, err := os.Stat(marker)
			assert.True(t, os.IsNotExist(err), "the denied call must not run")
			files, _ := filepath.Glob(filepath.Join(cfg, "projects", "*", "s-dw.jsonl"))
			require.Len(t, files, 1)
			raw, _ := os.ReadFile(files[0])
			res := toolResults(t, string(raw))
			assert.Equal(t, "PreToolUse:Bash hook error: DENY-REASON", res["t1"]["content"])
		})
	}
}
