package replay

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScriptCalls(t *testing.T) {
	r := newJSRun()
	calls, err := r.script("const r = await tools.exec_command({\"cmd\":`echo \\\"a\\\" ${1}`,'yield_time_ms':0}); text(r.output); const s = await tools.multi_agent_v1__spawn_agent({message:\"hi\"}); store(\"id\", s.agent_id);")
	require.NoError(t, err)
	require.Len(t, calls, 2)
	assert.Equal(t, "exec_command", calls[0].Name)
	assert.Equal(t, map[string]any{"cmd": `echo "a" 1`, "yield_time_ms": number{0}}, calls[0].Args[0])
	assert.Equal(t, "multi_agent_v1__spawn_agent", calls[1].Name)

	// the stored receipt names the spawn by its number in the rollout, in a later script
	calls, err = r.script("await tools.multi_agent_v1__wait_agent({targets:[load(\"id\")], timeout_ms:5});")
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"targets": []any{ref{call: 1, path: ".agent_id"}}, "timeout_ms": number{5}}, calls[0].Args[0])
}

func TestScriptLooksAroundWithoutCalls(t *testing.T) {
	calls, err := newJSRun().script("const hits = ALL_TOOLS.filter(x => /spawn_agent/i.test(x.name+\" \"+x.description)); text(JSON.stringify({hits}));")
	require.NoError(t, err)
	assert.Empty(t, calls)
}

func TestScriptRefusesWhatItCannotFollow(t *testing.T) {
	for name, js := range map[string]string{
		"a call that depends on a result": "const r = await tools.exec_command({cmd:\"a\"}); if (r.output) await tools.exec_command({cmd:\"b\"});",
		"an unknown name":                 "await tools.exec_command({cmd:nope});",
		"a loop":                          "for (const x of [1]) { await tools.exec_command({cmd:\"a\"}); }",
		"a load nobody stored":            "load(\"x\");",
		"not JavaScript":                  "const = ;",
		"a call inside a function":        "[1].map(async () => tools.exec_command({cmd:\"a\"}));",
		"a tools alias":                   "const t = tools; await t.exec_command({cmd:\"a\"});",
		"a shadowed harness name":         "const tools = {}; await tools.exec_command({cmd:\"a\"});",
		"a var":                           "{ var x = 1; } await tools.exec_command({cmd:x});",
		"a tagged template":               "await tools.exec_command`x`;",
		"a default parameter":             "[1].map((a = tools.exec_command({cmd:\"x\"})) => a);",
		"a mutating method":               "const a = [1]; a.reverse(); await tools.exec_command({cmd:\"c\", extra:a});",
		"an exponent-form number":         "await tools.exec_command({cmd:`${1e21}`});",
		"a prototype property":            "const o = {}; await tools.exec_command({cmd:o.toString});",
		"__proto__":                       "const o = {__proto__:{cmd:\"a\"}}; await tools.exec_command(o);",
		"a redeclaration":                 "const x = 1; const x = 2;",
		"a store that may not run":        "const r = await tools.exec_command({cmd:\"a\"}); if (r.output) { store(\"k\", 1); }",
	} {
		_, err := newJSRun().script(js)
		assert.Error(t, err, name)
	}
}

func TestScriptNamesAreScoped(t *testing.T) {
	calls, err := newJSRun().script("const x = \"a\"; { const x = \"b\"; await tools.exec_command({cmd:x}); } await tools.exec_command({cmd:x}); const o = {cmd:\"c\"}; await tools.exec_command({cmd:o[\"cmd\"]});")
	require.NoError(t, err)
	require.Len(t, calls, 3)
	assert.Equal(t, "b", calls[0].Args[0].(map[string]any)["cmd"])
	assert.Equal(t, "a", calls[1].Args[0].(map[string]any)["cmd"])
	assert.Equal(t, "c", calls[2].Args[0].(map[string]any)["cmd"])
}

func TestScriptCallsInMethodArguments(t *testing.T) {
	calls, err := newJSRun().script("text(JSON.stringify(await tools.exec_command({cmd:\"a\"}))); ALL_TOOLS.slice(await tools.exec_command({cmd:\"b\"}));")
	require.NoError(t, err)
	assert.Len(t, calls, 2)
}

func TestUnifyTakesOneArgument(t *testing.T) {
	_, err := unify(jsCall{Name: "exec_command", Args: []any{map[string]any{"cmd": "a"}, map[string]any{"cmd": "b"}}})
	assert.ErrorContains(t, err, "2 arguments")
}

func TestUnifyRefusesWhatItDrops(t *testing.T) {
	_, err := unify(jsCall{Name: "exec_command", Args: []any{map[string]any{"cmd": "a", "stdin": "x"}}})
	assert.ErrorContains(t, err, "stdin")
	_, err = unify(jsCall{Name: "exec_command", Args: []any{map[string]any{"cmd": "a", "yield_time_ms": opaque{}}}})
	assert.Error(t, err)
	_, err = unify(jsCall{Name: "exec_command", Args: []any{map[string]any{"cmd": "a", "workdir": "<RUN>"}}})
	assert.NoError(t, err)
}
