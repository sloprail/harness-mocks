package e2e

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"

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

	layout := regexp.MustCompile(`^sessions/\d{4}/\d{2}/\d{2}/rollout-\d{4}-\d{2}-\d{2}T\d{2}-\d{2}-\d{2}-(.+)\.jsonl$`)
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
			assert.Equal(t, l["session_id"], m[1], "%s: the file is keyed by the session id", p.name)
			assert.NotContains(t, rel, "repo", "%s: not keyed by the working directory", p.name)
		}
	}

	first := jsonLines(got.rollout(t))[0]
	assert.Equal(t, "session_meta", first["type"])
	assert.Equal(t, got.Repo, first["payload"].(map[string]any)["cwd"], "the working directory is a field of the first record")
}
