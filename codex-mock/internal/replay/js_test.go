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
	} {
		_, err := newJSRun().script(js)
		assert.Error(t, err, name)
	}
}
