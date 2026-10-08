package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// frameOrder is the stream's hook frames and message frames in order, each as
// "type/subtype/hook_name" (hook_name for a hook frame).
func frameOrder(stream string) (out []string) {
	for _, l := range strings.Split(stream, "\n") {
		var f map[string]any
		if json.Unmarshal([]byte(l), &f) != nil || f["type"] == nil {
			continue
		}
		typ := f["type"].(string)
		sub, _ := f["subtype"].(string)
		switch {
		case typ == "system" && (sub == "hook_started" || sub == "hook_response"):
			out = append(out, sub+"/"+f["hook_name"].(string))
		case typ == "assistant" && thinkingOnly(f):
			// the model's thinking is no frame of the mock's
		case typ == "assistant" || typ == "user" || typ == "result":
			out = append(out, typ)
		}
	}
	return
}

// thinkingOnly is whether an assistant frame holds nothing but thinking blocks.
func thinkingOnly(f map[string]any) bool {
	msg, _ := f["message"].(map[string]any)
	blocks, _ := msg["content"].([]any)
	for _, b := range blocks {
		if blk, _ := b.(map[string]any); blk["type"] != "thinking" {
			return false
		}
	}
	return len(blocks) > 0
}

// --include-hook-events adds a hook_started and a hook_response frame for each
// hook of the main thread, in firing order: UserPromptSubmit's, then each tool
// call's PreToolUse and PostToolUse frames, the PostToolUse ones ahead of the
// result frame, then Stop's, ahead of the run's result (runs/include-hook-events).
// Without the flag only SessionStart's are streamed.
// sr:proves noninteractive-run/claude
func TestT001_14_IncludeHookEventsStreamsEveryMainThreadHook(t *testing.T) {
	raw, err := os.ReadFile(recordedFile(t, filepath.Join("..", "..", "snapshots", "runs", "include-hook-events", "samples", "*", "stream.jsonl")))
	require.NoError(t, err)
	want := frameOrder(string(raw))
	require.Contains(t, want, "hook_started/PostToolUse:Bash")

	dir := t.TempDir()
	hook := filepath.Join(dir, "hook.sh")
	require.NoError(t, os.WriteFile(hook, []byte("#!/bin/sh\ncat >/dev/null\n"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".claude"), 0o755))
	var h []string
	for _, ev := range []string{"SessionStart", "UserPromptSubmit", "PreToolUse", "PostToolUse", "Stop"} {
		h = append(h, `"`+ev+`":[{"matcher":"*","hooks":[{"type":"command","command":"`+hook+`"}]}]`)
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".claude", "settings.json"), []byte(`{"hooks":{`+strings.Join(h, ",")+`}}`), 0o644))
	script := filepath.Join(dir, "s.sh")
	require.NoError(t, os.WriteFile(script, []byte(`#!/bin/sh
n=$(grep -c '"type":"tool_result"' "$A10N_MOCK_SESSION_FILE")
if [ "$n" = 0 ]; then echo '{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"t0","name":"Bash","input":{"command":"echo X"}}]}}'; exit 0; fi
echo '{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"DONE"}]}}'
echo '{"type":"result","subtype":"success","result":"DONE"}'
`), 0o755))
	out, code := runInDirWithEnv(t, dir, nil, "--script", script, "--session-id", "ihe-2", "--project-dir", dir, "--output-format", "stream-json", "--include-hook-events", "-p", "go")
	require.Equal(t, 0, code, out)
	assert.Equal(t, want, frameOrder(out))

	plain, code := runInDirWithEnv(t, dir, nil, "--script", script, "--session-id", "ihe-3", "--project-dir", dir, "--output-format", "stream-json", "-p", "go")
	require.Equal(t, 0, code, plain)
	assert.NotContains(t, plain, "hook_started/PreToolUse", "without the flag only SessionStart's frames stream")
	assert.NotContains(t, frameOrder(plain), "hook_started/PreToolUse:Bash")
}
