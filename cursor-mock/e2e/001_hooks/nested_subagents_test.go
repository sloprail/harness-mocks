package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The recorded runs runs/nested-subagents (the main agent starts a sub-agent
// that starts its own) and runs/nested-subagents-depth (a chain of four: the
// third layer is not offered the tool).

// layerScript is the scenario script of one layer: it starts the sub-agent whose
// script is next with a Task call, then says LEAF; with no next it runs the
// shell command echo LEAF and then says LEAF.
func layerScript(next string) string {
	say := `printf '%s\n' '{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"LEAF"}]}}' '{"type":"result","subtype":"success","result":"LEAF"}'`
	if next == "" {
		return `#!/bin/sh
n=$(grep -c '"type":"tool_use"' "$A10N_MOCK_SESSION_FILE" 2>/dev/null)
if [ "${n:-0}" = 0 ]; then
  printf '%s\n' '{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"call_sh","name":"Bash","input":{"command":"echo LEAF"}}]}}'
  exit 0
fi
` + say + "\n"
	}
	return fmt.Sprintf(`#!/bin/sh
n=$(grep -c '"type":"tool_use"' "$A10N_MOCK_SESSION_FILE" 2>/dev/null)
if [ "${n:-0}" = 0 ]; then
  printf '%%s\n' '{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"call_1","name":"Task","input":{"description":"next layer","prompt":"go","subagent_type":"generalPurpose","script":"%s"}}]}}'
  exit 0
fi
%s
`, next, say)
}

// nestedRun is one mock run on the recorded run's hooks (logging every
// preToolUse), whose main agent starts a chain of `layers` sub-agents.
type nestedRun struct {
	home, log string
	frames    []string
}

func runChain(t *testing.T, layers int) nestedRun {
	t.Helper()
	ws, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	scratch, home := t.TempDir(), t.TempDir()
	setup, _, _, _ := recording(t, "nested-subagents")
	copyFile(t, filepath.Join(setup, "hooks.json"), filepath.Join(ws, ".cursor", "hooks.json"), 0o644)
	copyFile(t, filepath.Join(setup, "hook.sh"), filepath.Join(ws, ".cursor", "hooks", "hook.sh"), 0o755)
	next := ""
	for i := layers; i >= 0; i-- {
		p := filepath.Join(scratch, fmt.Sprintf("layer%d.sh", i))
		require.NoError(t, os.WriteFile(p, []byte(layerScript(next)), 0o755))
		next = p
	}
	r := nestedRun{home: home, log: filepath.Join(scratch, "payloads.jsonl")}
	cmd := exec.Command(binary, "-p", "--force", "--trust", "--output-format", "stream-json", "--script", next, "go")
	cmd.Dir, cmd.Env = ws, []string{"PATH=" + os.Getenv("PATH"), "HOME=" + home, "HOOK_LOG=" + r.log}
	out, err := cmd.Output()
	require.NoError(t, err, "the mock failed: %s", out)
	for _, l := range strings.Split(string(out), "\n") {
		var f map[string]any
		if json.Unmarshal([]byte(l), &f) == nil && frameName(f) != "" {
			r.frames = append(r.frames, frameName(f))
		}
	}
	return r
}

// transcripts are the conversations' ids, from their files.
func (r nestedRun) transcripts(t *testing.T) (ids []string, dirs map[string]bool) {
	t.Helper()
	m, err := filepath.Glob(filepath.Join(r.home, ".cursor", "projects", "*", "agent-transcripts", "*", "*.jsonl"))
	require.NoError(t, err)
	dirs = map[string]bool{}
	for _, f := range m {
		ids = append(ids, filepath.Base(filepath.Dir(f)))
		dirs[filepath.Dir(filepath.Dir(f))] = true
	}
	sort.Strings(ids)
	return ids, dirs
}

// subagentEvents are the subagentStart and subagentStop hooks that fired.
func subagentEvents(hooks []map[string]any) (out []string) {
	for _, h := range hooks {
		if e, _ := h["hook_event_name"].(string); e == "subagentStart" || e == "subagentStop" {
			out = append(out, e)
		}
	}
	return
}

// taskSessions are the sessions the Task preToolUse hooks fired in.
func taskSessions(hooks []map[string]any) (ids []string) {
	for _, h := range hooks {
		if h["hook_event_name"] == "preToolUse" && h["tool_name"] == "Task" {
			ids = append(ids, h["session_id"].(string))
		}
	}
	return
}

// A sub-agent starts sub-agents of its own, each a conversation with its own
// transcript, all in the one directory of the session's: the recorded run has
// three, the main agent's and two sub-agents'. The Task call of each fires the
// preToolUse hook in the session of the agent making it (the sub-agent's own
// id, not the main agent's), and only the main agent's call is on the stream
// (runs/nested-subagents).
// sr:proves nested-subagents/cursor
func TestSubAgentsStartSubAgentsInTranscriptsOfTheirOwn(t *testing.T) {
	sample := newestSample(t, "nested-subagents")
	var recIDs []string
	dirs := map[string]bool{}
	m, err := filepath.Glob(filepath.Join(sample, "transcript", "*", "*.jsonl"))
	require.NoError(t, err)
	for _, f := range m {
		recIDs = append(recIDs, filepath.Base(filepath.Dir(f)))
		dirs[filepath.Dir(filepath.Dir(f))] = true
	}
	require.Len(t, recIDs, 3)
	require.Len(t, dirs, 1)
	recHooks := readJSONL(t, filepath.Join(sample, "payloads.jsonl"))
	assert.Empty(t, subagentEvents(recHooks), "configured, the subagentStart and subagentStop hooks did not fire")
	recSessions := taskSessions(recHooks)
	require.Len(t, recSessions, 2)
	assert.NotEqual(t, recSessions[0], recSessions[1], "the sub-agent's call is in its own session")
	assert.Contains(t, recIDs, recSessions[1], "which is a transcript of its own")
	_, rec, _, _ := recording(t, "nested-subagents")

	got := runChain(t, 2)
	ids, gotDirs := got.transcripts(t)
	assert.Len(t, ids, 3)
	assert.Len(t, gotDirs, 1, "all in one directory")
	assert.Empty(t, subagentEvents(readJSONL(t, got.log)), "nor in the mock")
	gotSessions := taskSessions(readJSONL(t, got.log))
	require.Len(t, gotSessions, 2)
	assert.NotEqual(t, gotSessions[0], gotSessions[1])
	assert.Contains(t, ids, gotSessions[1])
	assert.Equal(t, rec.frames, got.frames, "the main agent's calls only")

	// a sub-agent's transcript is a file like the main agent's: the one file of
	// its directory, with no sidecar and no depth or parent in its records
	for name, root := range map[string]string{"recorded": filepath.Join(sample, "transcript"),
		"mock": filepath.Join(got.home, ".cursor", "projects")} {
		files := 0
		require.NoError(t, filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			files++
			assert.True(t, strings.HasSuffix(p, ".jsonl"), "%s: %s is not a sidecar", name, p)
			for _, rec := range readJSONL(t, p) {
				for k := range rec {
					assert.Contains(t, []string{"role", "message", "type", "status"}, k, "%s: no depth or parent in %s", name, p)
				}
			}
			return nil
		}))
		assert.Equal(t, 3, files, name)
	}
}

// Nesting stops at a depth limit, two layers of sub-agents: a sub-agent at the
// limit is not offered the Task tool, so a chain of four agents under the main
// one has transcripts for the main agent and two sub-agents and no third
// (runs/nested-subagents-depth).
// sr:proves nested-subagents/cursor
func TestDepthLimitStopsNesting(t *testing.T) {
	sample := newestSample(t, "nested-subagents-depth")
	m, err := filepath.Glob(filepath.Join(sample, "transcript", "*", "*.jsonl"))
	require.NoError(t, err)
	require.Len(t, m, 3, "the main agent and two layers of sub-agents: the third could not start one")

	got := runChain(t, 3)
	ids, _ := got.transcripts(t)
	assert.Len(t, ids, 3)
	assert.Len(t, taskSessions(readJSONL(t, got.log)), 2, "the third layer's Task call is not a tool call the hooks see")
	assert.Empty(t, subagentEvents(readJSONL(t, filepath.Join(sample, "payloads.jsonl"))), "no sub-agent hooks in the recording")
	assert.Empty(t, subagentEvents(readJSONL(t, got.log)), "nor in the mock")
}
