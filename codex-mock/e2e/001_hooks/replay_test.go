package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const runsDir = "../../snapshots/runs"

// recording is one recorded sample of a real `codex exec` run.
type recording struct {
	setup  string   // runs/<name>/setup
	sample string   // runs/<name>/samples/<ts>
	calls  []string // the shell commands the model asked for, in order
}

func loadRecording(t *testing.T, name string) recording {
	t.Helper()
	samples, err := filepath.Glob(filepath.Join(runsDir, name, "samples", "*"))
	require.NoError(t, err)
	require.NotEmpty(t, samples, "no recorded sample of run %s", name)
	rec := recording{setup: filepath.Join(runsDir, name, "setup"), sample: samples[len(samples)-1]}
	seen := map[string]bool{}
	for _, p := range jsonLines(readFile(t, filepath.Join(rec.sample, "payloads.jsonl"))) {
		id, _ := p["tool_use_id"].(string)
		if p["hook_event_name"] == "PreToolUse" && !seen[id] {
			seen[id] = true
			in, _ := p["tool_input"].(map[string]any)
			rec.calls = append(rec.calls, in["command"].(string))
		}
	}
	if len(rec.calls) == 0 { // no before-tool hook logged: the commands that ran are the calls
		rec.calls, _ = result{Stdout: readFile(t, filepath.Join(rec.sample, "stream.jsonl"))}.commands()
	}
	return rec
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return ""
	}
	require.NoError(t, err)
	return string(b)
}

// replay runs the mock on the recorded run's own setup (its hooks.json, its
// hook script, its prompt), with a script that makes the calls the model made.
func replay(t *testing.T, rec recording, extraEnv ...string) result {
	t.Helper()
	return execMock(t, scenario{
		HooksJSON: readFile(t, filepath.Join(rec.setup, "hooks.json")),
		Files:     map[string]string{"hook.sh": readFile(t, filepath.Join(rec.setup, "hook.sh"))},
		Script:    callThenResult,
		Prompt:    strings.TrimSpace(readFile(t, filepath.Join(rec.setup, "prompt.txt"))),
		Env:       append(withCalls(t, rec.calls...), extraEnv...),
	})
}

// normalized is what a hook payload shows of what a scenario did, without what
// differs between two runs of the same behaviour (ids, paths, timings, the
// model's name, the OS's wording of an error): the same removals capture.sh makes.
func normalized(v any, sid string) any {
	switch x := v.(type) {
	case map[string]any:
		for _, k := range []string{"transcript_path", "cwd", "turn_id", "tool_use_id", "uuid", "timestamp", "duration_ms", "last_assistant_message", "model"} {
			delete(x, k)
		}
		if in, ok := x["tool_input"].(map[string]any); ok {
			delete(in, "description")
		}
		for k, e := range x {
			x[k] = normalized(e, sid)
		}
	case string:
		if strings.Contains(x, "No such file or directory") {
			return "<ENOENT>"
		}
		return strings.ReplaceAll(x, sid, "<SESSION_ID>")
	}
	return v
}

func sortedHookLines(lines []map[string]any) []string {
	var sid string
	for _, l := range lines {
		if s, ok := l["session_id"].(string); ok {
			sid = s
			break
		}
	}
	var out []string
	for _, l := range lines {
		b, _ := json.Marshal(normalized(l, sid))
		out = append(out, string(b))
	}
	sort.Strings(out)
	return out
}

// Every recorded run of the hooks a `codex exec` fires, replayed: the mock's
// hooks see the same payloads, and the hook scripts take the same exits; the
// event stream shows the same commands, run to the same exit statuses and
// outputs, in the same order (a command a hook refused is in neither).
// sr:proves hook-exit-code-semantics/codex
// sr:proves pretooluse-refusal/codex
func TestReplayOfRecordedRuns(t *testing.T) {
	for _, name := range []string{"stops", "hook-exit-codes", "hook-exit-json", "hook-unstartable",
		"pretool-decisions", "prompt-blocked", "posttool-block"} {
		t.Run(name, func(t *testing.T) {
			rec := loadRecording(t, name)
			got := replay(t, rec)
			require.Equal(t, 0, got.Code, got.Stderr)

			var want []map[string]any
			for _, l := range jsonLines(readFile(t, filepath.Join(rec.sample, "payloads.jsonl"))) {
				want = append(want, l)
			}
			assert.Equal(t, sortedHookLines(want), sortedHookLines(got.hookLog()), "hook payloads and exits")

			recStream := result{Stdout: readFile(t, filepath.Join(rec.sample, "stream.jsonl"))}
			wantCmds, wantExits := recStream.commands()
			gotCmds, gotExits := got.commands()
			assert.Equal(t, wantCmds, gotCmds, "commands that ran")
			assert.Equal(t, wantExits, gotExits, "their exit statuses")
			assert.Equal(t, streamShape(recStream.stream()), streamShape(got.stream()), "event stream")
		})
	}
}

// streamShape is the event types of a stream, without the warnings and the
// messages that depend on the run's flags and the model's words.
func streamShape(events []map[string]any) []string {
	var out []string
	for _, e := range events {
		typ, _ := e["type"].(string)
		item, _ := e["item"].(map[string]any)
		if item["type"] == "error" || item["type"] == "agent_message" {
			continue
		}
		if item != nil {
			typ += ":" + item["type"].(string)
		}
		out = append(out, typ)
	}
	return out
}
