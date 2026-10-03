package e2e

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestT017_78_ResumeByNameAsRecorded: a session started with --name is resumed
// by `--resume <name>`, as the resume-name run did: the same session continues in
// its own file, its transcript opens on a custom-title and an agent-name record
// holding the name, and the resume's SessionStart carries session_title, which an
// unnamed session's resume does not (forkresume).
// sr:proves session-resume/claude
func TestT017_78_ResumeByNameAsRecorded(t *testing.T) {
	raw, err := os.ReadFile(recordedFile(t, "../../snapshots/runs/resume-name/samples/*/events.jsonl"))
	require.NoError(t, err)
	var recordedTitle string
	for _, l := range strings.Split(string(raw), "\n") {
		var e struct {
			Hook    string
			Payload map[string]any
		}
		if json.Unmarshal([]byte(l), &e) == nil && e.Hook == "SessionStart" {
			assert.Equal(t, "resume", e.Payload["source"])
			recordedTitle, _ = e.Payload["session_title"].(string)
		}
	}
	require.Equal(t, "earlier-by-name", recordedTitle, "recorded: the resume carries the name")
	realRecs := readRecs(t, recordedFile(t, "../../snapshots/runs/resume-name/samples/*/transcript/*.jsonl"))
	assert.Contains(t, realRecs[0].Raw, `"custom-title"`)
	assert.Contains(t, realRecs[1].Raw, `"agent-name"`)

	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	log := filepath.Join(dir, "payloads.log")
	settings(t, dir, map[string]string{"SessionStart": payloadLogger(t, dir, "log.sh", log, "")})
	for _, args := range [][]string{
		{"--session-id", "plain-1"},
		{"--session-id", "named-1", "--name", "earlier-by-name"},
		{"--resume", "earlier-by-name"},
		{"--resume", "plain-1"},
	} {
		out, code := runInDir(t, dir, nil, append([]string{"--script", script(t, dir, "s"), "--project-dir", dir, "--config-dir", cfg, "-p", "go"}, args...)...)
		require.Equal(t, 0, code, out)
	}
	ps := payloads(t, log)
	require.Len(t, ps, 4)
	assert.Equal(t, "resume", ps[2]["source"])
	assert.Equal(t, "named-1", ps[2]["session_id"], "the name finds the named session")
	assert.Equal(t, recordedTitle, ps[2]["session_title"])
	assert.NotContains(t, ps[3], "session_title", "an unnamed session's resume carries no title")

	recs := readRecs(t, transcriptPath(t, cfg, dir, "named-1"))
	assert.Contains(t, recs[0].Raw, `"customTitle":"earlier-by-name"`)
	assert.Contains(t, recs[1].Raw, `"agentName":"earlier-by-name"`)
	files, err := filepath.Glob(filepath.Join(filepath.Dir(transcriptPath(t, cfg, dir, "named-1")), "*.jsonl"))
	require.NoError(t, err)
	assert.Len(t, files, 2, "resuming by name made no new transcript")
}

// TestT017_79_ResumeCostFactsArePricedAsRecorded: a resume's estimated_cache_write_usd
// is its context_tokens at 2 dollars per million, to the hundredth of a cent, in
// every recorded resume (forkresume, compact, resume-continue, resume-path,
// resume-name); the mock, which counts no tokens, estimates the context from the
// transcript's size and prices it the same way.
// sr:proves session-resume/claude
func TestT017_79_ResumeCostFactsArePricedAsRecorded(t *testing.T) {
	round4 := func(v float64) float64 { return math.Floor(v*10000+0.5) / 10000 }
	seen := 0
	for _, run := range []string{"forkresume", "compact", "resume-continue", "resume-path", "resume-name"} {
		raw, err := os.ReadFile(recordedFile(t, "../../snapshots/runs/"+run+"/samples/*/events.jsonl"))
		require.NoError(t, err)
		for _, l := range strings.Split(string(raw), "\n") {
			var e struct {
				Hook    string
				Payload map[string]any
			}
			if json.Unmarshal([]byte(l), &e) != nil || e.Hook != "SessionStart" || e.Payload["context_tokens"] == nil {
				continue
			}
			seen++
			tokens := e.Payload["context_tokens"].(float64)
			assert.Greater(t, tokens, float64(0), run)
			assert.Equal(t, round4(tokens*2/1e6), e.Payload["estimated_cache_write_usd"], "%s: %v tokens", run, tokens)
		}
	}
	require.GreaterOrEqual(t, seen, 5)

	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	log := filepath.Join(dir, "payloads.log")
	settings(t, dir, map[string]string{"SessionStart": payloadLogger(t, dir, "log.sh", log, "")})
	out, code := runInDir(t, dir, nil, "--script", script(t, dir, "a", toolUse("b1", "Bash", `{"command":"echo some output to fill the context"}`)), "--session-id", "cost-1",
		"--project-dir", dir, "--config-dir", cfg, "-p", "first")
	require.Equal(t, 0, code, out)
	info, err := os.Stat(transcriptPath(t, cfg, dir, "cost-1"))
	require.NoError(t, err)
	out, code = runInDir(t, dir, nil, "--script", script(t, dir, "b"), "--resume", "cost-1",
		"--project-dir", dir, "--config-dir", cfg, "-p", "again")
	require.Equal(t, 0, code, out)
	ps := payloads(t, log)
	require.Len(t, ps, 2)
	tokens := ps[1]["context_tokens"].(float64)
	assert.EqualValues(t, info.Size()/4, tokens, "the context is estimated from the transcript's size")
	assert.Equal(t, round4(tokens*2/1e6), ps[1]["estimated_cache_write_usd"])
}
