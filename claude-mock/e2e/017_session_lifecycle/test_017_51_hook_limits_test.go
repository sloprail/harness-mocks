package e2e

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestT017_51_PromptHookThatTimesOutAddsNoContext: a prompt hook that reaches
// its timeout is cancelled and its output, its context included, is discarded:
// the prompt still reaches the agent, without that context, and the run does
// not wait for the hook (docs, UserPromptSubmit).
// sr:proves hook-timeout/claude
func TestT017_51_PromptHookThatTimesOutAddsNoContext(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	slow := write(t, filepath.Join(dir, "ups.sh"), `#!/bin/sh
cat >/dev/null
sleep 30
echo '{"hookSpecificOutput":{"hookEventName":"UserPromptSubmit","additionalContext":"TOO-LATE"}}'
`, 0o755)
	fast := write(t, filepath.Join(dir, "fast.sh"), `#!/bin/sh
cat >/dev/null
echo '{"hookSpecificOutput":{"hookEventName":"UserPromptSubmit","additionalContext":"IN-TIME"}}'
`, 0o755)
	write(t, filepath.Join(dir, ".claude", "settings.json"),
		`{"hooks":{"UserPromptSubmit":[{"hooks":[{"type":"command","command":"`+slow+`","timeout":1},{"type":"command","command":"`+fast+`"}]}]}}`, 0o644)
	sc := write(t, filepath.Join(dir, "s.sh"), "#!/bin/sh\nprintf %s \"$A10N_MOCK_PROMPT|$A10N_MOCK_ADDITIONAL_CONTEXT\" > "+filepath.Join(dir, "seen.out")+"\necho '{\"type\":\"result\",\"subtype\":\"success\",\"result\":\"done\"}'\n", 0o755)
	start := time.Now()
	out, code := runInDir(t, dir, nil, "--script", sc, "--session-id", "pt-1",
		"--project-dir", dir, "--config-dir", cfg, "-p", "the prompt")
	require.Equal(t, 0, code, out)
	assert.Less(t, time.Since(start), 10*time.Second)
	seen, err := os.ReadFile(filepath.Join(dir, "seen.out"))
	require.NoError(t, err)
	assert.Equal(t, "the prompt|IN-TIME", string(seen), "the sibling's context stands, the cancelled hook's is discarded, the prompt is unchanged")
}

// TestT017_52_StopBlockCapForms: the cap bounds a Stop hook that blocks by exit
// status as it does one that blocks by JSON (2 blocks allowed: 3 fires, the
// third overridden), and a cap of 0 removes it (twelve blocks, thirteen fires,
// the turn ends when the hook lets it, with no override warning). Each block
// is handed back to the agent as feedback (docs, Stop decision control; env-vars:
// CLAUDE_CODE_STOP_HOOK_BLOCK_CAP).
// sr:proves stop-block-cap/claude
// sr:proves stop-block-continuation/claude
func TestT017_52_StopBlockCapForms(t *testing.T) {
	for _, tc := range []struct {
		name, cap string
		blocks    int // how many fires the hook blocks, then lets the turn end
		fires     int
		override  bool
	}{
		{"exit 2 with cap 2", "2", 1000, 3, true},
		{"cap 0 is unlimited", "0", 12, 13, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			cfg := filepath.Join(dir, "config")
			log := filepath.Join(dir, "payloads.log")
			counter := filepath.Join(dir, "n")
			hook := payloadLogger(t, dir, "stop.sh", log, `N=$(cat `+counter+` 2>/dev/null || echo 0); N=$((N+1)); echo $N > `+counter+`
if [ $N -le `+strconv.Itoa(tc.blocks)+` ]; then echo "KEEP-GOING-$N" >&2; exit 2; fi`)
			settings(t, dir, map[string]string{"Stop": hook})
			sc := write(t, filepath.Join(dir, "s.sh"), `#!/bin/sh
echo '{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"DONE"}]}}'
echo '{"type":"result","subtype":"success","result":"DONE"}'
`, 0o755)
			out, code := runInDir(t, dir, []string{"CLAUDE_CODE_STOP_HOOK_BLOCK_CAP=" + tc.cap}, "--script", sc,
				"--session-id", "cf-1", "--project-dir", dir, "--config-dir", cfg, "-p", "hello")
			require.Equal(t, 0, code, out)
			assert.Len(t, payloads(t, log), tc.fires)
			assert.Equal(t, 1, strings.Count(out, `"type":"result"`), "one result at the turn's real end")
			raw, err := os.ReadFile(transcriptPath(t, cfg, dir, "cf-1"))
			require.NoError(t, err)
			assert.Equal(t, tc.override, strings.Contains(string(raw), "overriding and ending turn"))
			assert.Contains(t, string(raw), "Stop hook feedback:\\n["+hook+"]: KEEP-GOING-1\\n", "each block is handed back as feedback")
		})
	}
}
