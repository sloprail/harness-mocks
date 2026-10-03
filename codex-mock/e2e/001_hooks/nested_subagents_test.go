package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The recorded runs runs/nested-subagents (the main thread spawns a sub-agent
// that spawns its own, max_depth 3), runs/nested-subagents-limit (a third
// layer, max_depth 2) and runs/nested-subagents-nowait.

// nestedRecording is a recorded run's setup and its latest sample (these runs' calls
// are not shell commands, so they have no calls list).
func nestedRecording(t *testing.T, name string) recording {
	t.Helper()
	samples, err := filepath.Glob(filepath.Join(runsDir, name, "samples", "*"))
	require.NoError(t, err)
	require.NotEmpty(t, samples, "no recorded sample of run %s", name)
	return recording{setup: filepath.Join(runsDir, name, "setup"), sample: samples[len(samples)-1]}
}

// chainScript is a sub-agent's scenario script: it spawns the sub-agent whose
// script is next (the mock's spawn_agent waits for it) and says LEAF; with no
// next it runs the shell command echo LEAF and says LEAF. A spawn_agent the
// harness answers as an unknown tool leaves nothing to run.
func chainScript(next string) string {
	spawn := `{"message":"go","script":"` + next + `"}`
	say := `printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"text","text":"LEAF"}]}}' '{"type":"result","subtype":"success","result":"LEAF"}'`
	if next == "" {
		return `#!/bin/sh
n=$(grep -c function_call_output "$A10N_MOCK_SESSION_FILE")
if [ "$n" = 0 ]; then
  printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"tool_use","id":"call_0","name":"Bash","input":{"command":"echo LEAF"}}]}}'
  exit 0
fi
` + say + "\n"
	}
	return fmt.Sprintf(`#!/bin/sh
n=$(grep -c function_call_output "$A10N_MOCK_SESSION_FILE")
if [ "$n" = 0 ]; then
  printf '{"type":"assistant","message":{"content":[{"type":"tool_use","id":"call_0","name":"spawn_agent","input":%%s}]}}\n' '%s'
  exit 0
fi
%s
`, spawn, say)
}

// chain runs the mock on the recorded run's setup with a main-thread script
// that spawns a chain of sub-agents `layers` deep, under the given extra args.
func chain(t *testing.T, name string, layers int, args ...string) result {
	t.Helper()
	dir := t.TempDir()
	next := ""
	for i := layers; i >= 1; i-- {
		p := filepath.Join(dir, fmt.Sprintf("layer%d.sh", i))
		require.NoError(t, os.WriteFile(p, []byte(chainScript(next)), 0o755))
		next = p
	}
	rec := nestedRecording(t, name)
	return execMock(t, scenario{
		HooksJSON: readFile(t, filepath.Join(rec.setup, "hooks.json")),
		Files:     map[string]string{"hook.sh": readFile(t, filepath.Join(rec.setup, "hook.sh"))},
		Script:    chainScript(next),
		Prompt:    "go",
		Args:      args,
	})
}

// thread is one rollout's meta record, the thread's own (id) and where it came from.
type thread struct {
	ID, Parent string
	Depth      float64 // 0 for the main thread
	Calls      []string
	Told       []string // what its tool calls were answered
}

// threads are the rollouts under dir (a session's home, mock's or recorded),
// in the one directory they all share.
func threads(t *testing.T, root string) (out []thread, dirs map[string]bool) {
	t.Helper()
	dirs = map[string]bool{}
	require.NoError(t, filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasPrefix(info.Name(), "rollout-") {
			return nil
		}
		dirs[filepath.Dir(p)] = true
		th := thread{}
		for _, rec := range jsonLines(readFile(t, p)) {
			pl, _ := rec["payload"].(map[string]any)
			switch {
			case rec["type"] == "session_meta":
				th.ID, _ = pl["id"].(string)
				th.Parent, _ = pl["parent_thread_id"].(string)
				spawn, _ := pl["source"].(map[string]any)
				if sub, ok := spawn["subagent"].(map[string]any); ok {
					th.Depth, _ = sub["thread_spawn"].(map[string]any)["depth"].(float64)
				}
			case pl["type"] == "function_call_output":
				out, _ := pl["output"].(string)
				th.Told = append(th.Told, out)
			case pl["type"] == "function_call" || pl["type"] == "custom_tool_call":
				name, _ := pl["name"].(string)
				th.Calls = append(th.Calls, name)
			}
		}
		out = append(out, th)
		return nil
	}))
	return
}

// byDepth indexes the threads of a chain by depth, and requires one each.
func byDepth(t *testing.T, ths []thread) map[float64]thread {
	m := map[float64]thread{}
	for _, th := range ths {
		_, dup := m[th.Depth]
		require.False(t, dup, "two threads at depth %v", th.Depth)
		m[th.Depth] = th
	}
	return m
}

// A sub-agent spawns sub-agents of its own: each thread records its depth
// (one for the main thread's sub-agent, two for its) and the thread that
// spawned it, and all of a session's rollouts sit in one directory. The hooks
// of a sub-agent's calls name it (agent_id), the main thread's do not, and
// only the main thread's calls are in the event stream
// (runs/nested-subagents).
// sr:proves nested-subagents/codex
func TestSubAgentsSpawnSubAgentsAndRecordDepthAndParent(t *testing.T) {
	rec := nestedRecording(t, "nested-subagents")
	wantThreads, wantDirs := threads(t, filepath.Join(rec.sample, "transcript"))
	require.Len(t, wantThreads, 3)
	require.Len(t, wantDirs, 1)

	got := chain(t, "nested-subagents", 2, "-c", "agents.max_depth=3")
	require.Equal(t, 0, got.Code, got.Stderr)
	gotThreads, gotDirs := threads(t, filepath.Join(got.Home, "sessions"))
	assert.Len(t, gotDirs, 1, "all the rollouts of a session in one directory")
	for name, ths := range map[string][]thread{"recorded": wantThreads, "mock": gotThreads} {
		d := byDepth(t, ths)
		require.Len(t, d, 3, name)
		assert.Empty(t, d[0].Parent, name)
		assert.Equal(t, d[0].ID, d[1].Parent, "%s: a depth 1 sub-agent's parent is the main thread", name)
		assert.Equal(t, d[1].ID, d[2].Parent, "%s: a depth 2 sub-agent's parent is the sub-agent that spawned it", name)
	}

	// who the hooks name: nothing for the main thread's calls, the sub-agent
	// (agent_type default) for its own, for every tool it uses; PostToolUse
	// fires after the multi-agent calls as after the shell, with a response.
	// (The mock's spawn_agent waits, so the wait tool's own hooks are left out.)
	who := func(lines []map[string]any, ths []thread) (out []string) {
		role := map[string]string{}
		for _, th := range ths {
			role[th.ID] = fmt.Sprintf("depth%v", th.Depth)
		}
		for _, l := range lines {
			if tool, _ := l["tool_name"].(string); strings.Contains(tool, "wait") {
				continue
			}
			w := "main"
			if id, ok := l["agent_id"].(string); ok {
				w = role[id] + ":" + l["agent_type"].(string)
			}
			out = append(out, fmt.Sprintf("%v %v %v response=%v", l["hook_event_name"], l["tool_name"], w, l["tool_response"] != nil))
		}
		sort.Strings(out)
		return out
	}
	want := who(jsonLines(readFile(t, filepath.Join(rec.sample, "payloads.jsonl"))), wantThreads)
	assert.Equal(t, want, who(got.hookLog(), gotThreads))
	assert.Contains(t, want, "PostToolUse spawn_agent depth1:default response=true", "the recording shows the call's after-hook")

	// a sub-agent's hooks carry the main session's id as their session_id, not
	// the sub-agent's own (the doc: "Subagent hooks use the parent session id")
	for name, p := range map[string]struct {
		lines []map[string]any
		ths   []thread
	}{
		"recorded": {jsonLines(readFile(t, filepath.Join(rec.sample, "payloads.jsonl"))), wantThreads},
		"mock":     {got.hookLog(), gotThreads},
	} {
		main := byDepth(t, p.ths)[0].ID
		subs := 0
		for _, l := range p.lines {
			if _, ok := l["agent_id"]; ok {
				subs++
				assert.Equal(t, main, l["session_id"], "%s: a sub-agent's hook names the main session", name)
			}
		}
		assert.Positive(t, subs, name)
	}

	recStream := result{Stdout: readFile(t, filepath.Join(rec.sample, "stream.jsonl"))}
	assert.Equal(t, streamShape(recStream.stream()), streamShape(got.stream()), "the main thread's calls only")
}

// Nesting stops at a depth limit, agents.max_depth (one when it is not set):
// a sub-agent at the limit is not offered the spawn tool, and a call of it is
// answered as the unknown tool it is, so no thread is spawned below it. The
// recorded run, max_depth 2, has sub-agents at depths one and two and none at
// three (runs/nested-subagents-limit).
// sr:proves nested-subagents/codex
func TestDepthLimitStopsNesting(t *testing.T) {
	rec := nestedRecording(t, "nested-subagents-limit")
	wantThreads, _ := threads(t, filepath.Join(rec.sample, "transcript"))
	require.Len(t, byDepth(t, wantThreads), 3, "the main thread and sub-agents at depth 1 and 2: no depth 3")

	for _, tc := range []struct {
		name string
		args []string
		// depths that exist in the mock under the limit
		want []float64
	}{
		{"max_depth 2", []string{"-c", "agents.max_depth=2"}, []float64{0, 1, 2}},
		{"default", nil, []float64{0, 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := chain(t, "nested-subagents-limit", 3, tc.args...)
			require.Equal(t, 0, got.Code, got.Stderr)
			ths, _ := threads(t, filepath.Join(got.Home, "sessions"))
			d := byDepth(t, ths)
			var depths []float64
			for k := range d {
				depths = append(depths, k)
			}
			assert.ElementsMatch(t, tc.want, depths)
			// the deepest layer that could not spawn was told so
			deepest := d[tc.want[len(tc.want)-1]]
			assert.NotContains(t, deepest.Calls, "wait_agent")
			assert.Contains(t, deepest.Told, "unsupported call: spawn_agent")
		})
	}
}
