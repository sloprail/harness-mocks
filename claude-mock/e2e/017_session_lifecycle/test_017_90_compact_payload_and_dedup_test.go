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

// The PreCompact and PostCompact payloads carry the common input fields of any
// event (session_id, transcript_path, cwd and the prompt) and no
// permission_mode, as the recorded compact run's do, with the recorded set of keys
// (runs/compact).
// sr:proves manual-compaction/claude
func TestT017_90_CompactPayloadsCarryTheCommonFields(t *testing.T) {
	var want = map[string][]string{}
	for _, p := range recordedHooks(t, "compact") {
		if ev, _ := p["hook_event_name"].(string); ev == "PreCompact" || ev == "PostCompact" {
			want[ev] = keysOf(p)
		}
	}
	require.Len(t, want, 2)

	dir := t.TempDir()
	cfg, log := filepath.Join(dir, "config"), filepath.Join(dir, "payloads.log")
	h := payloadLogger(t, dir, "log.sh", log, "")
	compactSettings(t, dir, map[string][2]string{"PreCompact": {"*", h}, "PostCompact": {"*", h}})
	compactRun(t, dir, cfg, "cmp-fields", "manual")
	seen := map[string][]string{}
	for _, p := range payloads(t, log) {
		ev := p["hook_event_name"].(string)
		seen[ev] = keysOf(p)
		assert.NotContains(t, p, "permission_mode", ev)
		assert.Equal(t, "cmp-fields", p["session_id"], ev)
		assert.NotEmpty(t, p["transcript_path"], ev)
		assert.NotEmpty(t, p["cwd"], ev)
	}
	assert.Equal(t, want["PreCompact"], seen["PreCompact"])
	assert.Equal(t, want["PostCompact"], seen["PostCompact"])
}

// The same hook command listed under the same matcher in two of a project's
// settings files runs once, not once per file (docs, hook handler fields;
// recorded in runs/hook-two-settings-files: one run of the hook for the two files).
// sr:proves hook-matcher-filter/claude
// sr:proves hooks-all-matching-run/claude
func TestT017_90_ASameHookInTwoSettingsFilesRunsOnce(t *testing.T) {
	rec, err := os.ReadFile(recordedFile(t, "../../snapshots/runs/hook-two-settings-files/samples/*/payloads.jsonl"))
	require.NoError(t, err)
	assert.Equal(t, 1, strings.Count(string(rec), `"hook_ran":"same"`), "recorded: the hook ran once for the two files")

	dir := t.TempDir()
	log := filepath.Join(dir, "payloads.log")
	h := payloadLogger(t, dir, "log.sh", log, "")
	entry := func(extra string) string {
		b, err := json.Marshal(map[string]any{"hooks": map[string]any{"Stop": []any{map[string]any{"matcher": "*", "hooks": []any{map[string]any{"type": "command", "command": h}}}}}})
		require.NoError(t, err)
		return string(b) + extra
	}
	write(t, filepath.Join(dir, ".claude", "settings.json"), entry(""), 0o644)
	write(t, filepath.Join(dir, ".claude", "settings.local.json"), entry(""), 0o644)
	out, code := runInDir(t, dir, nil, "--script", script(t, dir, "s"), "--session-id", "dedup-1",
		"--project-dir", dir, "--config-dir", filepath.Join(dir, "config"), "-p", "hello")
	require.Equal(t, 0, code, out)
	assert.Len(t, payloads(t, log), 1, "one Stop payload: the hook ran once")
}
