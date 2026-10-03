package e2e

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// A test here replays a recorded cursor-agent run against the mock: the run's
// own setup (its hooks.json, hook scripts, prompt and env) is installed in a
// fresh workspace, the scenario script plays back the tool calls the real
// agent made (its tool_call started frames), and what the mock's hooks saw
// and what its stream shows is compared with what the recording shows. The
// recording is read from the run's newest committed sample.

// observed is what a run showed: the payloads its hooks read, in order, what
// the hook scripts logged of their own decisions, and the tool-call frames.
type observed struct {
	hooks   []map[string]any
	results []string
	frames  []string
	// raw are the payloads as the hooks read them, and ws and home the
	// workspace and the home the run used (set on the mock's side only).
	raw []map[string]any
	// envs are the environments the hook scripts logged of themselves.
	envs     []map[string]any
	ws, home string
	// stdout is the mock's whole stream, as printed (set on the mock's side only).
	stdout string
}

// dropped are the fields a capture's normalizer removes (capture.sh): what
// differs between two runs of the same behaviour.
var dropped = map[string]bool{
	"transcript_path": true, "cwd": true, "workspace_roots": true, "user_email": true, "generation_id": true,
	"tool_use_id": true, "duration": true, "duration_ms": true, "model": true, "model_id": true, "model_params": true,
	"cursor_version": true, "conversation_id": true,
}

// normalize is capture.sh's normalizer: the dropped fields out, the session
// id as <SESSION_ID> and the workspace as <RUN>.
func normalize(v any, sid, ws string) any {
	switch x := v.(type) {
	case map[string]any:
		out := map[string]any{}
		for k, e := range x {
			if !dropped[k] {
				out[k] = normalize(e, sid, ws)
			}
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = normalize(e, sid, ws)
		}
		return out
	case string:
		return strings.ReplaceAll(strings.ReplaceAll(x, sid, "<SESSION_ID>"), ws, "<RUN>")
	}
	return v
}

// unmodeledEnv drops from a recorded payload the two variables a shell
// command saw that the mock does not set (CURSOR_REQUEST_ID and
// CURSOR_RIPGREP_PATH: adr/modeled-surface).
func unmodeledEnv(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := map[string]any{}
		for k, e := range x {
			out[k] = unmodeledEnv(e)
		}
		return out
	case string:
		for _, name := range []string{"CURSOR_REQUEST_ID", "CURSOR_RIPGREP_PATH"} {
			x = strings.ReplaceAll(strings.ReplaceAll(x, name+"\n", ""), name+`\n`, "")
		}
		return x
	}
	return v
}

func readJSONL(t *testing.T, path string) []map[string]any {
	t.Helper()
	f, err := os.Open(path)
	require.NoError(t, err)
	defer f.Close()
	var out []map[string]any
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		var m map[string]any
		if json.Unmarshal(sc.Bytes(), &m) == nil && m != nil {
			out = append(out, m)
		}
	}
	return out
}

// frameName is how a tool_call or result frame is named for comparison.
func frameName(f map[string]any) string {
	typ, _ := f["type"].(string)
	sub, _ := f["subtype"].(string)
	if typ == "result" {
		return "result/" + sub
	}
	tc, _ := f["tool_call"].(map[string]any)
	for k, v := range tc {
		if !strings.HasSuffix(k, "ToolCall") {
			continue
		}
		outcome := ""
		if res, ok := v.(map[string]any)["result"].(map[string]any); ok {
			for rk := range res {
				if rk != "isBackground" {
					outcome = rk
				}
			}
		}
		return "tool_call/" + sub + "/" + k + "/" + outcome
	}
	return ""
}

// resultName names what a hook script logged of its own decision: its event
// and the exit status it gave (afterAgentThought, which the mock does not
// fire, is left out).
func resultName(m map[string]any) string {
	if _, ok := m["hook_ran"]; ok {
		return fmt.Sprintf("ran:%v:%v:%v:%v:%v", m["hook_ran"], m["event"], m["tool"], m["command"], m["phase"])
	}
	r, ok := m["hook_result"].(map[string]any)
	if !ok || r["event"] == "afterAgentThought" {
		return ""
	}
	if s, ok := r["script"].(string); ok {
		return fmt.Sprintf("%s:%v:%v", s, r["event"], r["command"])
	}
	return fmt.Sprintf("%v:%v", r["event"], r["exit"])
}

func jsonString(v any) string { b, _ := json.Marshal(v); return string(b) }

// recording is a run's newest committed sample.
func recording(t *testing.T, run string) (setup string, rec observed, calls []string, prompt string) {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(file), "..", "..", "snapshots", "runs", run)
	samples, err := filepath.Glob(filepath.Join(root, "samples", "*"))
	require.NoError(t, err)
	require.NotEmpty(t, samples, "run %s has no sample", run)
	sort.Strings(samples)
	sample := samples[len(samples)-1]
	for _, e := range readJSONL(t, filepath.Join(sample, "events.jsonl")) {
		p, _ := e["payload"].(map[string]any)
		switch {
		case e["event"] == "stream":
			if n := frameName(map[string]any{"type": e["type"], "subtype": e["subtype"], "tool_call": toolCall(e)}); n != "" {
				rec.frames = append(rec.frames, n)
			}
		case e["hook"] == "beforeReadFile" || e["hook"] == "afterAgentThought":
			// events the mock does not fire (adr/modeled-surface)
		case e["hook"] != nil:
			rec.hooks = append(rec.hooks, unmodeledEnv(p).(map[string]any))
		case p["hook_env"] != nil:
			rec.envs = append(rec.envs, hookEnv(p["hook_env"], ""))
		case p["hook_result"] != nil || p["hook_ran"] != nil:
			if n := resultName(p); n != "" {
				rec.results = append(rec.results, n)
			}
		}
	}
	sort.Strings(rec.results)
	b, err := os.ReadFile(filepath.Join(sample, "stream.jsonl"))
	require.NoError(t, err)
	for _, l := range strings.Split(string(b), "\n") {
		var f map[string]any
		if json.Unmarshal([]byte(l), &f) == nil && f["type"] == "tool_call" && f["subtype"] == "started" {
			calls = append(calls, l)
		}
	}
	p, err := os.ReadFile(filepath.Join(root, "setup", "prompt.txt"))
	require.NoError(t, err)
	return filepath.Join(root, "setup"), rec, calls, strings.TrimSpace(string(p))
}

// toolCall rebuilds the frame shape frameName reads from a normalized stream
// event ({event, type, subtype, tool, outcome}).
func toolCall(e map[string]any) map[string]any {
	tool, _ := e["tool"].(string)
	if tool == "" {
		return nil
	}
	body := map[string]any{}
	if o, _ := e["outcome"].(string); o != "" {
		body["result"] = map[string]any{o: true}
	}
	return map[string]any{tool: body}
}

func copyFile(t *testing.T, from, to string, mode os.FileMode) {
	t.Helper()
	b, err := os.ReadFile(from)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(to), 0o755))
	require.NoError(t, os.WriteFile(to, b, mode))
}

// replay runs the recorded scenario against the mock and returns what it
// showed beside what the recording showed.
func replay(t *testing.T, run string) (got, want observed) {
	t.Helper()
	setup, want, calls, prompt := recording(t, run)
	ws, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	scratch := t.TempDir()
	copyFile(t, filepath.Join(setup, "hooks.json"), filepath.Join(ws, ".cursor", "hooks.json"), 0o644)
	scripts, _ := filepath.Glob(filepath.Join(setup, "*.sh"))
	for _, s := range scripts {
		copyFile(t, s, filepath.Join(ws, ".cursor", "hooks", filepath.Base(s)), 0o755)
	}
	writes := writtenContents(want)
	for i, c := range calls {
		line, rest := scriptCall(t, strings.ReplaceAll(c, "<RUN>", ws), writes)
		writes = rest
		require.NoError(t, os.WriteFile(filepath.Join(scratch, itoa(i)+".json"), []byte(line+"\n"), 0o644))
	}
	require.NoError(t, os.WriteFile(filepath.Join(scratch, "end.json"), []byte(`{"type":"result","subtype":"success","is_error":false,"result":"DONE"}`+"\n"), 0o644))
	script := filepath.Join(scratch, "scenario.sh")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\nn=$(grep -c '\"type\":\"tool_use\"' \"$A10N_MOCK_SESSION_FILE\" 2>/dev/null)\nn=${n:-0}\nf=\""+scratch+"/$n.json\"\n[ -f \"$f\" ] || f=\""+scratch+"/end.json\"\ncat \"$f\"\n"), 0o755))
	logPath := filepath.Join(scratch, "payloads.jsonl")
	home := t.TempDir()
	env := []string{"PATH=" + os.Getenv("PATH"), "HOME=" + home, "HOOK_LOG=" + logPath, "TMPDIR=" + scratch, "A10N_MOCK_SCRIPT=" + script}
	if b, err := os.ReadFile(filepath.Join(setup, "env")); err == nil {
		for _, l := range strings.Split(string(b), "\n") {
			if l != "" {
				env = append(env, l)
			}
		}
	}
	flags := []string{"-p", "--force", "--trust", "--output-format", "stream-json"}
	if _, err := os.Stat(filepath.Join(setup, "no-force")); err == nil { // a run captured without --force
		flags = []string{"-p", "--trust", "--output-format", "stream-json"}
	}
	cmd := exec.Command(binary, append(flags, prompt)...)
	cmd.Dir, cmd.Env = ws, env
	out, err := cmd.Output()
	require.NoError(t, err, "the mock failed: %s", out)

	var sid string
	for _, m := range readJSONL(t, logPath) {
		if s, ok := m["session_id"].(string); ok && sid == "" {
			sid = s
		}
	}
	for _, m := range readJSONL(t, logPath) {
		switch {
		case m["hook_event_name"] != nil && m["hook_event_name"] != "afterAgentThought":
			got.hooks = append(got.hooks, normalize(m, sid, ws).(map[string]any))
			got.raw = append(got.raw, m)
		case m["hook_env"] != nil:
			got.envs = append(got.envs, hookEnv(m["hook_env"], ws))
		case m["hook_result"] != nil || m["hook_ran"] != nil:
			if n := resultName(m); n != "" {
				got.results = append(got.results, n)
			}
		}
	}
	sort.Strings(got.results)
	got.ws, got.home, got.stdout = ws, home, string(out)
	for _, l := range strings.Split(string(out), "\n") {
		var f map[string]any
		if json.Unmarshal([]byte(l), &f) == nil {
			if n := frameName(f); n != "" {
				got.frames = append(got.frames, n)
			}
		}
	}
	return got, want
}

// writtenContents are the contents the recorded run's Write calls wrote, in
// order, as its preToolUse hook saw them. A started frame's streamContent is
// only what the model had streamed so far, not always what ended in the file.
func writtenContents(rec observed) (out []string) {
	for _, h := range rec.hooks {
		if h["hook_event_name"] == "preToolUse" && h["tool_name"] == "Write" {
			out = append(out, h["tool_input"].(map[string]any)["content"].(string))
		}
	}
	return out
}

// scriptCall is the line the scenario script prints for a tool call the
// recorded agent made: the scenario protocol's assistant line with one tool_use
// block (the Claude Code names, which the mock maps onto Cursor's tools). A
// write's content is what the recorded hooks saw written (the started frame's
// streamContent is only what the model had streamed so far); writes are the
// recorded contents not yet used, and what is left after this call is returned.
func scriptCall(t *testing.T, frame string, writes []string) (string, []string) {
	t.Helper()
	var f map[string]any
	require.NoError(t, json.Unmarshal([]byte(frame), &f))
	id, _ := f["call_id"].(string)
	var name string
	var input map[string]any
	for kind, v := range f["tool_call"].(map[string]any) {
		body, ok := v.(map[string]any)
		if !ok || !strings.HasSuffix(kind, "ToolCall") {
			continue
		}
		args := body["args"].(map[string]any)
		switch kind {
		case "shellToolCall":
			name, input = "Bash", map[string]any{"command": args["command"]}
		case "readToolCall":
			name, input = "Read", map[string]any{"file_path": args["path"]}
		case "editToolCall":
			content, _ := args["streamContent"].(string)
			if len(writes) > 0 {
				content, writes = writes[0], writes[1:]
			}
			name, input = "Write", map[string]any{"file_path": args["path"], "content": content}
		}
	}
	require.NotEmpty(t, name, "a started frame naming no tool the mock runs: %s", frame)
	return jsonString(map[string]any{"type": "assistant", "message": map[string]any{"role": "assistant", "content": []any{
		map[string]any{"type": "tool_use", "id": id, "name": name, "input": input}}}}), writes
}

// hookEnv is what a hook logged of its environment, as the recording shows it:
// the workspace as <RUN>, without the path of cursor-agent's ripgrep, which the
// mock does not set, and without the transcript path, which cursor-agent hands
// a hook or not depending on a race (the last hook always has it).
func hookEnv(v any, ws string) map[string]any {
	out := map[string]any{}
	for k, e := range v.(map[string]any) {
		s, _ := e.(string)
		switch k {
		case "CURSOR_RIPGREP_PATH":
			continue
		case "CURSOR_TRANSCRIPT_PATH":
			continue // whether the file is named yet when a hook starts is a race in cursor-agent
		default:
			if ws != "" {
				s = strings.ReplaceAll(s, ws, "<RUN>")
			}
		}
		out[k] = s
	}
	return out
}

func itoa(i int) string { return strings.TrimSpace(jsonString(i)) }
