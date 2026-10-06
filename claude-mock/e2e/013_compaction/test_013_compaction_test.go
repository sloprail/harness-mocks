package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeSessionStartHook installs a SessionStart hook that:
//   - appends the "source" field of every SessionStart fire to logFile, and
//   - on source="compact" outputs additionalContext (so the mock surfaces it as a
//     system record on the output stream, mirroring real claude's compact re-seed).
func writeSessionStartHook(t *testing.T, dir, logFile string) {
	t.Helper()
	hook := filepath.Join(dir, "session_start.sh")
	require.NoError(t, os.WriteFile(hook, []byte(`#!/bin/sh
input=$(cat)
src=$(echo "$input" | grep -o '"source":"[^"]*"' | head -1 | cut -d'"' -f4)
echo "$src" >> "`+logFile+`"
if [ "$src" = "compact" ]; then
  printf '%s\n' '{"hookSpecificOutput":{"hookEventName":"SessionStart","additionalContext":"REINJECTED-MEMORY"}}'
fi
`), 0o755))
	claudeDir := filepath.Join(dir, ".claude")
	require.NoError(t, os.MkdirAll(claudeDir, 0o755))
	s := `{"hooks":{"SessionStart":[{"matcher":"*","hooks":[{"type":"command","command":"` + hook + `"}]}]}}`
	require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(s), 0o644))
}

// compactionScenario emits `n` compaction records — shaped like the record real
// Claude Code writes when it auto-compacts the context window
// ({"type":"user","isCompactSummary":true,…}) — followed by a final result frame,
// all in a single script invocation. The runner reacts to each compaction record by
// firing SessionStart source="compact". The compaction record itself carries NO
// additionalContext; the SessionStart compact hook supplies it.
func compactionScenario(n int) string {
	var b strings.Builder
	b.WriteString("#!/bin/sh\n")
	for i := 0; i < n; i++ {
		b.WriteString(`printf '%s\n' '{"type":"user","isCompactSummary":true,"message":{"role":"user","content":"[context compacted]"}}'` + "\n")
	}
	b.WriteString(`printf '%s\n' '{"type":"result","subtype":"success","result":"done","is_error":false}'` + "\n")
	return b.String()
}

// TestT013_01_CompactionRecordFiresSessionStartCompact: when the scenario emits a
// compaction record, the runner fires SessionStart with source="compact" and the
// hook's additionalContext is surfaced on the output stream — the mechanism by which
// a plugin re-seeds a compacted context window. ScheduleWakeup is NOT involved.
// sr:proves session-start-hook/claude
func TestT013_01_CompactionRecordFiresSessionStartCompact(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "sources.txt")
	writeSessionStartHook(t, dir, logFile)

	script := filepath.Join(dir, "scenario.sh")
	require.NoError(t, os.WriteFile(script, []byte(compactionScenario(1)), 0o755))

	out, code := runInDir(t, dir, nil, "--script", script, "--session-id", "s-compact", "--project-dir", dir, "-p", "go")
	require.Equal(t, 0, code, "compaction-record run must succeed\noutput:\n%s", out)

	// The compaction record was forwarded to the output stream (part of history).
	assert.Contains(t, out, `"isSynthetic":true`,
		"the compaction summary must reach the output stream as a synthetic user frame\noutput:\n%s", out)

	// SessionStart fired at startup AND on compact.
	data, err := os.ReadFile(logFile)
	require.NoError(t, err, "SessionStart hook must have fired")
	assert.Contains(t, string(data), "startup", "SessionStart should fire at startup")
	assert.Contains(t, string(data), "compact",
		"a compaction record must fire SessionStart with source=compact\nsources:\n%s", string(data))

	// The compact SessionStart hook's additionalContext is surfaced on the stream.
	assert.Contains(t, out, "additionalContext",
		"the compact SessionStart additionalContext must be surfaced\noutput:\n%s", out)
	assert.Contains(t, out, "REINJECTED-MEMORY",
		"the compact hook's injected context must reach the output stream\noutput:\n%s", out)
}

// TestT013_02_MultipleCompactionsEachFireCompact: two compaction records in one run
// each fire SessionStart source="compact", and the hook's additionalContext is
// surfaced on each — proving compaction is a repeatable trajectory event.
// sr:proves session-start-hook/claude
func TestT013_02_MultipleCompactionsEachFireCompact(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "sources.txt")
	writeSessionStartHook(t, dir, logFile)

	script := filepath.Join(dir, "scenario.sh")
	require.NoError(t, os.WriteFile(script, []byte(compactionScenario(2)), 0o755))

	out, code := runInDir(t, dir, nil, "--script", script, "--session-id", "s-compact2", "--project-dir", dir, "-p", "go")
	require.Equal(t, 0, code, "output:\n%s", out)

	// SessionStart source=compact fired twice (once per compaction record).
	data, err := os.ReadFile(logFile)
	require.NoError(t, err)
	assert.Equal(t, 2, strings.Count(string(data), "compact"),
		"each compaction record must fire SessionStart source=compact (expected 2)\nsources:\n%s", string(data))

	// additionalContext surfaced on each compaction.
	assert.GreaterOrEqual(t, strings.Count(out, "REINJECTED-MEMORY"), 2,
		"the compact hook's context must be surfaced on each compaction (expected >=2)\noutput:\n%s", out)
}

// TestT013_03_CompactionRecordCarriesNoOwnContext: the compaction record itself
// carries no additionalContext — the SessionStart compact hook is the sole source.
// With a hook that emits nothing on compact, no additionalContext appears even though
// SessionStart source=compact still fires.
func TestT013_03_CompactionRecordCarriesNoOwnContext(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "sources.txt")

	// SessionStart hook that logs the source but NEVER emits additionalContext.
	hook := filepath.Join(dir, "session_start.sh")
	require.NoError(t, os.WriteFile(hook, []byte(`#!/bin/sh
input=$(cat)
src=$(echo "$input" | grep -o '"source":"[^"]*"' | head -1 | cut -d'"' -f4)
echo "$src" >> "`+logFile+`"
`), 0o755))
	claudeDir := filepath.Join(dir, ".claude")
	require.NoError(t, os.MkdirAll(claudeDir, 0o755))
	s := `{"hooks":{"SessionStart":[{"matcher":"*","hooks":[{"type":"command","command":"` + hook + `"}]}]}}`
	require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(s), 0o644))

	script := filepath.Join(dir, "scenario.sh")
	require.NoError(t, os.WriteFile(script, []byte(compactionScenario(1)), 0o755))

	out, code := runInDir(t, dir, nil, "--script", script, "--session-id", "s-compact3", "--project-dir", dir, "-p", "go")
	require.Equal(t, 0, code, "output:\n%s", out)

	// SessionStart source=compact still fired.
	data, err := os.ReadFile(logFile)
	require.NoError(t, err)
	assert.Contains(t, string(data), "compact",
		"compaction must fire SessionStart source=compact even with a silent hook\nsources:\n%s", string(data))

	// No additionalContext system record (the record carries none; the hook emits none).
	assert.NotContains(t, out, "additionalContext",
		"no additionalContext should appear when the compact hook emits none\noutput:\n%s", out)
}
