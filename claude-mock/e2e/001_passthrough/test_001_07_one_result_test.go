package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestT001_07_OneEventPerRecordAndOneResult: with --output-format stream-json a run
// streams one machine-readable event (a JSON object on a line of its own) per record
// the scripted agent produces, in order, and ends with exactly one result frame even
// when the turn is continued: a Stop hook that blocks twice makes the script run
// three times, and only the last turn's result is streamed (recorded: snapshots/runs/stops,
// "a turn that Stop re-prompts streams a single result frame, at its real end").
// sr:proves noninteractive-run/claude
func TestT001_07_OneEventPerRecordAndOneResult(t *testing.T) {
	dir := t.TempDir()
	count := filepath.Join(dir, "blocks")
	hook := writeScript(t, `#!/bin/sh
cat >/dev/null
n=$(cat `+count+` 2>/dev/null || echo 0)
if [ "$n" -lt 2 ]; then echo $((n+1)) > `+count+`; echo '{"decision":"block","reason":"again"}'; fi
`)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".claude"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".claude", "settings.json"),
		[]byte(`{"hooks":{"Stop":[{"matcher":"*","hooks":[{"type":"command","command":"`+hook+`"}]}]}}`), 0o644))
	script := writeScript(t, `#!/bin/sh
n=$(grep -c '"type":"assistant"' "$A10N_MOCK_SESSION_FILE" 2>/dev/null || true)
printf '{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"turn %s"}],"stop_reason":"end_turn"}}\n' "$n"
printf '{"type":"result","subtype":"success","result":"result of turn %s","is_error":false}\n' "$n"
`)
	out, code := runInDirWithEnv(t, dir, nil, "--script", script, "--session-id", "one-result",
		"--project-dir", dir, "--output-format", "stream-json", "--verbose", "-p", "go")
	require.Equal(t, 0, code, out)

	var types []string
	var results []string
	for _, line := range nonEmptyLines(out) {
		var rec map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &rec), "every line of the stream is one JSON event: %q", line)
		types = append(types, rec["type"].(string))
		if rec["type"] == "result" {
			results = append(results, rec["result"].(string))
		}
	}
	assert.Equal(t, []string{"assistant", "user", "system", "assistant", "user", "assistant", "result"}, types, "one event per assistant record and per block's feedback to the agent (the first block's error notice among them), then the one result")
	assert.Equal(t, []string{"result of turn 2"}, results, "the result of the turn's real end")
	lines := nonEmptyLines(out)
	var last map[string]any
	require.NoError(t, json.Unmarshal([]byte(lines[len(lines)-1]), &last))
	assert.Equal(t, "result", last["type"], "the stream ends with the result")
	assert.Equal(t, false, last["is_error"])
}
