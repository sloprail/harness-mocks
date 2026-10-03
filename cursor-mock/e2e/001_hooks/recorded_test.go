package e2e

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// newestSample is a run's newest committed sample directory.
func newestSample(t *testing.T, run string) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	samples, err := filepath.Glob(filepath.Join(filepath.Dir(file), "..", "..", "snapshots", "runs", run, "samples", "*"))
	require.NoError(t, err)
	require.NotEmpty(t, samples, "run %s has no sample", run)
	sort.Strings(samples)
	return samples[len(samples)-1]
}

// TestTheTranscriptFromASymlinkedDirectoryIsKeyedByTheRealPathAndAbsentAtStart:
// recorded (runs/symlinked-cwd: cursor-agent started from a symlink to the
// workspace, its hooks logging what they saw), the workspace is the resolved
// path (the stream's init cwd and the hook payloads' workspace_roots), the
// project folder under the config directory is named after that resolved path,
// never after the symlink, and no session transcript file exists when the
// sessionStart hook runs; the payloads' transcript_path is null, and the file
// is not there, at the start hook; from the first command's
// afterShellExecution on both are there; in between (the first preToolUse and
// beforeShellExecution) whether cursor-agent has named the file yet varies from
// one capture to the next, so only what the two always agree on is pinned. The
// project folder is keyed by the resolved path at every event, the start hook's
// included, and never by the symlink the process was started from. The mock
// names the path once the first preToolUse has run.
// sr:proves session-transcript-file/cursor
func TestTheTranscriptFromASymlinkedDirectoryIsKeyedByTheRealPathAndAbsentAtStart(t *testing.T) {
	dir := newestSample(t, "symlinked-cwd")
	stream := readJSONL(t, filepath.Join(dir, "stream.jsonl"))
	require.Equal(t, "<RUN>", stream[0]["cwd"], "the init frame names the resolved workspace, not the symlink")
	for _, f := range []string{"payloads.jsonl", "stream.jsonl"} {
		b, err := os.ReadFile(filepath.Join(dir, f))
		require.NoError(t, err)
		require.NotContains(t, string(b), "<LINK_DIRNAME>", f+" names the symlink's own folder")
	}

	// the recorded transition, event by event: no path in the payload and no file
	// at the start hook, both from the first command's afterShellExecution on
	seen := map[string]int{}
	named := map[string]bool{"afterShellExecution": true, "postToolUse": true, "sessionEnd": true}
	unnamed := map[string]bool{"sessionStart": true}
	for _, m := range readJSONL(t, filepath.Join(dir, "payloads.jsonl")) {
		if roots, ok := m["workspace_roots"].([]any); ok {
			require.Equal(t, []any{"<RUN>"}, roots)
		}
		if tp, _ := m["transcript_path"].(string); tp != "" {
			require.Contains(t, tp, "/.cursor/projects/<RUN_DIRNAME>/agent-transcripts/")
		}
		if ev, _ := m["hook_event_name"].(string); named[ev] || unnamed[ev] {
			if named[ev] {
				require.NotNil(t, m["transcript_path"], ev+": the payload names the transcript")
			} else {
				require.Nil(t, m["transcript_path"], ev+": the payload's transcript_path is null")
			}
		}
		r, ok := m["hook_result"].(map[string]any)
		if !ok || r["event"] == "afterAgentThought" {
			continue
		}
		require.Equal(t, "<RUN>", r["pwd_physical"], "a hook runs in the resolved workspace")
		ev, _ := r["event"].(string)
		seen[ev]++
		require.Equal(t, true, r["key_resolved"], ev+": the project folder is keyed by the resolved workspace")
		require.Equal(t, false, r["key_symlink"], ev+": and not by the symlink the process started from")
		if r["transcript_path_set"] == true {
			require.Equal(t, true, r["transcript_exists"], ev+": a payload that names the transcript names a file that is there")
		}
		switch {
		case unnamed[ev]:
			require.Equal(t, false, r["transcript_path_set"], ev)
			require.Equal(t, false, r["transcript_exists"], ev)
			require.EqualValues(t, 0, r["transcript_files"], ev+": no transcript file exists yet")
		case named[ev]:
			require.Equal(t, true, r["transcript_path_set"], ev)
			require.Equal(t, true, r["transcript_exists"], ev)
			require.EqualValues(t, 1, r["transcript_files"], ev)
		}
	}
	for _, ev := range []string{"sessionStart", "preToolUse", "beforeShellExecution", "afterShellExecution", "postToolUse", "sessionEnd"} {
		require.Equal(t, 1, seen[ev], ev+" fired once")
	}

	// the mock, started from a symlink, keys the transcript the same way, holds no
	// file at the start hook, and names the file in the payloads and has it by the
	// first command's afterShellExecution
	c := runCustomAt(t, true, `{"version":1,"hooks":{"sessionStart":[{"command":"cat >/dev/null; find \"$HOME/.cursor/projects\" -name '*.jsonl' | wc -l | tr -d ' ' > \"$HOOK_LOG.count\""}],"afterShellExecution":[{"command":"cat > \"$HOOK_LOG.after\"; find \"$HOME/.cursor/projects\" -name '*.jsonl' | wc -l | tr -d ' ' > \"$HOOK_LOG.aftercount\""}]}}`, nil, "echo hi")
	path, session := c.transcript(t)
	project := strings.NewReplacer("/", "-", ".", "-", "_", "-").Replace(strings.TrimPrefix(c.ws, "/"))
	require.Equal(t, filepath.Join(c.home, ".cursor", "projects", project, "agent-transcripts", session, session+".jsonl"), path, "keyed by the resolved path")
	n, _ := os.ReadFile(c.log + ".count")
	require.Equal(t, "0", strings.TrimSpace(string(n)), "no transcript file exists when the start hook runs")
	after, _ := os.ReadFile(c.log + ".after")
	require.Contains(t, string(after), `"transcript_path":"`+path+`"`, "afterShellExecution names the transcript")
	n, _ = os.ReadFile(c.log + ".aftercount")
	require.Equal(t, "1", strings.TrimSpace(string(n)), "the transcript file exists by afterShellExecution")
}

// TestValidJSONWithANonZeroStatusOtherThan2IsIgnored: recorded
// (runs/hook-json-nonzero), a preToolUse and a beforeShellExecution hook that
// print a valid JSON deny and exit 1 or 3 do not refuse the call: the status
// classifies the outcome (a non-blocking error) and the JSON is not read, so
// the command runs and its after-hooks fire.
// sr:proves hook-exit-code-semantics/cursor
func TestValidJSONWithANonZeroStatusOtherThan2IsIgnored(t *testing.T) {
	got, want := replay(t, "hook-json-nonzero")
	conforms(t, got, want)

	for _, r := range []string{"preToolUse:1", "beforeShellExecution:1", "preToolUse:3", "beforeShellExecution:3"} {
		require.Contains(t, want.results, r, "the recorded hook printed a JSON deny and exited with this status")
	}
	for _, cmd := range []string{"echo JSONEXIT1", "echo JSONEXIT3"} {
		require.Equal(t, "preToolUse beforeShellExecution afterShellExecution postToolUse", joined(eventsOf(got, cmd)), cmd)
		_, _, failed := failureOf(got, cmd)
		require.False(t, failed, cmd)
	}
	require.NotContains(t, got.frames, "tool_call/completed/shellToolCall/rejected")
}
