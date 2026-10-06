package e2e

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The recorded run runs/session-transcript-file: a session whose start and
// stop hooks log their payload and whether the file transcript_path names
// exists at that moment.

// A session's transcript is a file under the configuration directory, named by
// the session id and kept under the day the session started, not under the
// working directory; and it already exists when the start hook runs, with its
// first record naming the working directory (runs/session-transcript-file).
// sr:proves session-transcript-file/codex
func TestTranscriptIsADayAndSessionIDFileThatExistsAtStart(t *testing.T) {
	rec := loadRecording(t, "session-transcript-file")
	want := jsonLines(readFile(t, filepath.Join(rec.sample, "payloads.jsonl")))
	got := replay(t, rec)
	require.Equal(t, 0, got.Code, got.Stderr)

	probes := func(log []map[string]any) (out []map[string]any) {
		for _, l := range log {
			if _, ok := l["probe"]; ok {
				out = append(out, l)
			}
		}
		return
	}
	require.Len(t, probes(want), 2)
	assert.Equal(t, probes(want), probes(got.hookLog()), "the file exists at start and at stop")

	layout := regexp.MustCompile(`^sessions/(\d{4})/(\d{2})/(\d{2})/rollout-(\d{4})-(\d{2})-(\d{2})T\d{2}-\d{2}-\d{2}-(.+)\.jsonl$`)
	paths := map[string][]string{}
	for _, p := range []struct {
		name string
		log  []map[string]any
		home string
	}{{"recording", want, "<TMP>/home/.codex/"}, {"mock", got.hookLog(), got.Home + "/"}} {
		for _, l := range p.log {
			path, ok := l["transcript_path"].(string)
			if !ok {
				continue
			}
			rel := strings.TrimPrefix(path, p.home)
			m := layout.FindStringSubmatch(rel)
			require.NotNil(t, m, "%s: %s is not under the configuration directory as a dated rollout", p.name, path)
			assert.Equal(t, l["session_id"], m[7], "%s: the file is keyed by the session id", p.name)
			assert.Equal(t, m[1:4], m[4:7], "%s: the directory's day is the day in the file name", p.name)
			paths[p.name] = append(paths[p.name], path)
			assert.NotContains(t, rel, "repo", "%s: not keyed by the working directory", p.name)
		}
	}

	first := jsonLines(got.rollout(t))[0]
	assert.Equal(t, "session_meta", first["type"])
	assert.Equal(t, got.Repo, first["payload"].(map[string]any)["cwd"], "the working directory is a field of the first record")

	for name, ps := range paths {
		require.Len(t, ps, 2, name+": the start and the stop payload name a file")
		assert.Equal(t, ps[0], ps[1], name+": the same file at start and at stop")
	}
	var onDisk []string
	require.NoError(t, filepath.Walk(filepath.Join(got.Home, "sessions"), func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			onDisk = append(onDisk, p)
		}
		return nil
	}))
	assert.Equal(t, onDisk, paths["mock"][:1], "transcript_path names the file the session wrote")
	day := time.Now().Format("2006/01/02")
	assert.Contains(t, onDisk[0], "/sessions/"+day+"/", "kept under the day it started")
}

// Every event of a session names the same transcript file in its payload, tool
// hooks and SessionEnd included, and the file exists whenever the hook runs
// (runs/session-transcript-file-hooks).
// sr:proves session-transcript-file/codex
func TestEveryEventNamesTheSameExistingTranscript(t *testing.T) {
	rec := loadRecording(t, "session-transcript-file-hooks")
	want := jsonLines(readFile(t, filepath.Join(rec.sample, "payloads.jsonl")))
	got := replay(t, rec)
	require.Equal(t, 0, got.Code, got.Stderr)

	shape := func(log []map[string]any) (events []string, probes []any, paths map[string]bool) {
		paths = map[string]bool{}
		for _, l := range log {
			if p, ok := l["probe"]; ok {
				probes = append(probes, p, l["transcript_exists"])
				continue
			}
			events = append(events, l["hook_event_name"].(string))
			path, _ := l["transcript_path"].(string)
			paths[path] = true
		}
		return
	}
	wantEvents, wantProbes, wantPaths := shape(want)
	gotEvents, gotProbes, gotPaths := shape(got.hookLog())
	assert.Equal(t, []string{"SessionStart", "PreToolUse", "PostToolUse", "Stop", "SessionEnd"}, wantEvents)
	assert.Equal(t, wantEvents, gotEvents)
	assert.Equal(t, wantProbes, gotProbes, "the file exists whenever a hook runs, as recorded")
	assert.Len(t, wantPaths, 1, "recorded: one file for every event")
	assert.Len(t, gotPaths, 1, "the mock names one file for every event")
	for p := range gotPaths {
		assert.NotEmpty(t, p)
		assert.Equal(t, got.rollout(t), readFile(t, p), "and it is the session's transcript")
	}
}

// With --ephemeral a session keeps nothing, so every hook payload names no transcript: transcript_path
// is null on each event (the docs type it string | null, "if any"), and no session file is left under
// the configuration directory (recorded: runs/ephemeral-no-transcript). The mock does the same, and
// the script still reads the session so far.
// sr:proves hook-common-payload/codex
// sr:proves session-transcript-file/codex
// sr:proves noninteractive-run/codex
func TestAnEphemeralSessionHasNoTranscriptAndItsHooksSayNull(t *testing.T) {
	rec := loadRecording(t, "ephemeral-no-transcript")
	nulls := func(log []map[string]any) (events []string) {
		for _, l := range log {
			if ev, ok := l["hook_event_name"].(string); ok {
				assert.Contains(t, l, "transcript_path", ev)
				assert.Nil(t, l["transcript_path"], ev)
				events = append(events, ev)
			}
		}
		return
	}
	want := nulls(jsonLines(readFile(t, filepath.Join(rec.sample, "payloads.jsonl"))))
	assert.Equal(t, []string{"SessionStart", "PreToolUse", "PostToolUse", "Stop", "SessionEnd"}, want, "recorded")
	kept, _ := filepath.Glob(filepath.Join(rec.sample, "transcript", "*"))
	assert.Empty(t, kept, "recorded: no rollout is kept")

	got := execMock(t, scenario{
		HooksJSON: readFile(t, filepath.Join(rec.setup, "hooks.json")),
		Files:     map[string]string{"hook.sh": readFile(t, filepath.Join(rec.setup, "hook.sh"))},
		Script:    callThenResult, Prompt: "go", Env: withCalls(t, "echo one"), Args: []string{"--ephemeral"},
	})
	require.Equal(t, 0, got.Code, got.Stderr)
	assert.Equal(t, want, nulls(got.hookLog()), "the mock's")
	var files []string
	_ = filepath.Walk(filepath.Join(got.Home, "sessions"), func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			files = append(files, p)
		}
		return nil
	})
	assert.Empty(t, files, "the mock keeps no session file")
	// nothing is left anywhere after the run: the configuration directory holds no session, and the
	// scratch directory the mock kept the session in while it ran is gone from the temporary directory
	scratch, _ := filepath.Glob(filepath.Join(got.Tmp, "codex-mock-ephemeral-*"))
	assert.Empty(t, scratch, "the mock removes its scratch session")
	entries, _ := os.ReadDir(got.Home)
	for _, e := range entries {
		assert.NotEqual(t, "sessions", e.Name(), "the mock leaves no sessions directory under the configuration directory")
	}
	cmds, _ := got.commands()
	assert.Equal(t, []string{"echo one"}, cmds, "the script read the session so far and went on")
}
