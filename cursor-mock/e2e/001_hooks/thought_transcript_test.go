package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// transcriptNulls are, for each afterAgentThought of a run's payloads in order,
// whether it named no transcript.
func transcriptNulls(hooks []map[string]any) []bool {
	var nulls []bool
	for _, h := range hooks {
		if h["hook_event_name"] == "afterAgentThought" {
			nulls = append(nulls, h["transcript_path"] == nil)
		}
	}
	return nulls
}

// TestAThoughtBeforeTheTranscriptExistsNamesNoTranscript: recorded, an
// afterAgentThought carries a null transcript_path until the transcript file
// exists, as every other payload does: the first one after the first preToolUse
// in runs/tool-failure, and the first one, before any preToolUse, in
// runs/shell-exit-status; the later ones name the file. The mock's first thought,
// made before any call, names none, and the one after its call names the file.
// sr:proves hook-common-payload/cursor
func TestAThoughtBeforeTheTranscriptExistsNamesNoTranscript(t *testing.T) {
	for _, run := range []string{"tool-failure", "shell-exit-status"} {
		recorded := transcriptNulls(readJSONL(t, filepath.Join(newestSample(t, run), "payloads.jsonl")))
		require.NotEmpty(t, recorded, run)
		require.True(t, recorded[0], run+": recorded, the first thought names no transcript")
		require.False(t, recorded[len(recorded)-1], run+": recorded, the last one names it")
	}

	ws, home, scratch := t.TempDir(), t.TempDir(), t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(ws, ".cursor", "hooks"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(ws, ".cursor", "hooks.json"), []byte(`{"version":1,"hooks":{"afterAgentThought":[{"command":".cursor/hooks/log.sh"}]}}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(ws, ".cursor", "hooks", "log.sh"), []byte("#!/bin/sh\ncat >>\"$HOOK_LOG\"\necho >>\"$HOOK_LOG\"\n"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(ws, "note.txt"), []byte("hi\n"), 0o644))
	script := filepath.Join(scratch, "s.sh")
	require.NoError(t, os.WriteFile(script, []byte(`#!/bin/sh
n=$(grep -c '"type":"tool_use"' "$A10N_MOCK_SESSION_FILE" 2>/dev/null)
if [ "${n:-0}" = 0 ]; then
  printf '%s\n' '{"type":"assistant","message":{"role":"assistant","content":[{"type":"thinking","thinking":"Reading."},{"type":"tool_use","id":"c1","name":"Read","input":{"file_path":"note.txt"}}]}}'
else
  printf '%s\n' '{"type":"assistant","message":{"role":"assistant","content":[{"type":"thinking","thinking":"Done."},{"type":"text","text":"DONE"}]}}' '{"type":"result","subtype":"success","result":"DONE"}'
fi
`), 0o755))
	log := filepath.Join(scratch, "log.jsonl")
	cmd := exec.Command(binary, "-p", "--force", "--trust", "--output-format", "stream-json", "--script", script, "go")
	cmd.Dir, cmd.Env = ws, []string{"PATH=" + os.Getenv("PATH"), "HOME=" + home, "HOOK_LOG=" + log}
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	require.Equal(t, []bool{true, false}, transcriptNulls(readJSONL(t, log)), "the mock's thoughts: the first names no transcript, the last does")
}
