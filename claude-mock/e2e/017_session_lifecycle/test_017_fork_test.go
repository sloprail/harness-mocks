package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// forkRecs is a fork's records with the preamble dropped.
func forkRecs(t *testing.T, path string) []rec {
	var out []rec
	for _, r := range readRecs(t, path) {
		if r.UUID != "" {
			out = append(out, r)
		}
	}
	return out
}

// TestT017_08_ForkOfACompactedSession is the shape of all 19 real transcripts
// that open on a compact_boundary: a verbatim copy of the last boundary, the
// summary chained to it, then the records the boundary preserved, re-parented
// into one chain after the summary, then what followed the summary, and the
// fork's own turn — every record under the fork's sessionId. SessionStart
// fires with source "fork" (docs; claude 2.1.282). The original is untouched.
// staged:proves session-fork/claude
// sr:proves session-start-hook/claude
func TestT017_08_ForkOfACompactedSession(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	log := filepath.Join(dir, "payloads.log")
	settings(t, dir, map[string]string{"SessionStart": payloadLogger(t, dir, "log.sh", log, "")})
	sc := script(t, dir, "s",
		toolUse("b1", "Bash", `{"command":"true"}`),
		`{"type":"compact","summary":"compacted @MARK@","preserve":3}`,
	)
	out, code := runInDir(t, dir, nil, "--script", sc, "--session-id", "orig",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)
	origPath := transcriptPath(t, cfg, dir, "orig")
	before, err := os.ReadFile(origPath)
	require.NoError(t, err)
	for _, fork := range []string{"fork-a", "fork-b"} {
		out, code = runInDir(t, dir, nil, "--script", script(t, dir, fork), "--resume", "orig", "--fork-session",
			"--session-id", fork, "--project-dir", dir, "--config-dir", cfg, "-p", "continue "+fork)
		require.Equal(t, 0, code, out)
	}
	after, err := os.ReadFile(origPath)
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after), "forking leaves the original untouched")

	orig := readRecs(t, origPath)
	var boundary rec
	var preserved []string
	for _, r := range orig {
		if r.Subtype == "compact_boundary" {
			boundary = r
			var full map[string]any
			require.NoError(t, json.Unmarshal([]byte(r.Raw), &full))
			for _, u := range full["compactMetadata"].(map[string]any)["preservedMessages"].(map[string]any)["uuids"].([]any) {
				preserved = append(preserved, u.(string))
			}
		}
	}
	require.NotEmpty(t, boundary.UUID)
	require.Len(t, preserved, 3)
	for _, fork := range []string{"fork-a", "fork-b"} {
		recs := forkRecs(t, transcriptPath(t, cfg, dir, fork))
		require.GreaterOrEqual(t, len(recs), 5)
		assert.Equal(t, boundary.UUID, recs[0].UUID, "%s opens on the same boundary", fork)
		assert.Nil(t, recs[0].ParentUUID)
		assert.Equal(t, boundary.LogicalParentUUID, recs[0].LogicalParentUUID)
		assert.Contains(t, recs[1].Raw, `"isCompactSummary":true`, "then the summary")
		require.NotNil(t, recs[1].ParentUUID)
		assert.Equal(t, recs[0].UUID, *recs[1].ParentUUID)
		for k, u := range preserved {
			r := recs[2+k]
			assert.Equal(t, u, r.UUID, "then the preserved records, in order")
			require.NotNil(t, r.ParentUUID)
			assert.Equal(t, recs[1+k].UUID, *r.ParentUUID, "re-parented into one chain after the summary")
		}
		for i, r := range recs {
			assert.Equal(t, fork, r.SessionID, "record %d of %s carries the fork's session id", i, fork)
			if i > 0 {
				require.NotNil(t, r.ParentUUID, "record %d of %s", i, fork)
				assert.Equal(t, recs[i-1].UUID, *r.ParentUUID, "record %d of %s chains to the one before", i, fork)
			}
		}
		raw, _ := os.ReadFile(transcriptPath(t, cfg, dir, fork))
		assert.Contains(t, string(raw), "continue "+fork)
	}
	var sources []any
	for _, p := range payloads(t, log) {
		sources = append(sources, p["source"])
	}
	assert.Equal(t, []any{"startup", "compact", "fork", "fork"}, sources)
}

// TestT017_09_ForkOfAnUncompactedSession: a session never compacted forks
// whole — origin included, parents unchanged — under the fork's sessionId, as
// claude 2.1.282 forked one (and every 2.1.280+ fork on the machine). A fork
// of the fork works the same way.
// staged:proves session-fork/claude
// sr:proves session-start-hook/claude
func TestT017_09_ForkOfAnUncompactedSession(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	log := filepath.Join(dir, "payloads.log")
	settings(t, dir, map[string]string{"SessionStart": payloadLogger(t, dir, "log.sh", log, "")})
	out, code := runInDir(t, dir, nil, "--script", script(t, dir, "s", toolUse("b1", "Bash", `{"command":"true"}`)), "--session-id", "plain",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)
	origPath := transcriptPath(t, cfg, dir, "plain")
	before, _ := os.ReadFile(origPath)
	out, code = runInDir(t, dir, nil, "--script", script(t, dir, "f"), "--resume", "plain", "--fork-session",
		"--session-id", "plain-fork", "--project-dir", dir, "--config-dir", cfg, "-p", "more")
	require.Equal(t, 0, code, out)
	out, code = runInDir(t, dir, nil, "--script", script(t, dir, "g"), "--resume", "plain-fork", "--fork-session",
		"--session-id", "plain-fork-2", "--project-dir", dir, "--config-dir", cfg, "-p", "even more")
	require.Equal(t, 0, code, out)
	after, _ := os.ReadFile(origPath)
	assert.Equal(t, string(before), string(after), "forking leaves the original untouched")

	orig := forkRecs(t, origPath)
	for _, fork := range []string{"plain-fork", "plain-fork-2"} {
		recs := forkRecs(t, transcriptPath(t, cfg, dir, fork))
		require.Greater(t, len(recs), len(orig))
		for i, o := range orig {
			assert.Equal(t, o.UUID, recs[i].UUID, "%s copies record %d", fork, i)
			assert.Equal(t, o.ParentUUID, recs[i].ParentUUID, "%s keeps record %d's parent", fork, i)
			assert.Equal(t, fork, recs[i].SessionID, "%s's copy carries its own session id", fork)
		}
	}
	raw, _ := os.ReadFile(transcriptPath(t, cfg, dir, "plain-fork-2"))
	assert.Contains(t, string(raw), `"more"`, "the fork of the fork carries the first fork's turn")
	assert.Contains(t, string(raw), `"even more"`)
	var sources []any
	for _, p := range payloads(t, log) {
		sources = append(sources, p["source"])
	}
	assert.Equal(t, []any{"startup", "fork", "fork"}, sources)
}

// TestT017_10_CompactionCanNameAnUnwrittenLogicalParent: a real
// preserved-segment compaction named, as its logical parent, a record written
// to no transcript. The scenario can reproduce it.
// sr:proves control-records
func TestT017_10_CompactionCanNameAnUnwrittenLogicalParent(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	sc := script(t, dir, "s", `{"type":"compact","logical_parent":"never-written","summary":"x @MARK@"}`)
	out, code := runInDir(t, dir, nil, "--script", sc, "--session-id", "cmp-2",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)
	for _, r := range readRecs(t, transcriptPath(t, cfg, dir, "cmp-2")) {
		if r.Subtype == "compact_boundary" {
			assert.Equal(t, "never-written", r.LogicalParentUUID)
			var full map[string]any
			require.NoError(t, json.Unmarshal([]byte(r.Raw), &full))
			pm := full["compactMetadata"].(map[string]any)["preservedMessages"].(map[string]any)
			all := pm["allUuids"].([]any)
			assert.Equal(t, append(append([]any{}, pm["uuids"].([]any)...), "never-written"), all,
				"allUuids is uuids plus the unwritten record the boundary names (a strict superset, as in 44 of 65 real boundaries)")
			return
		}
	}
	t.Fatal("no boundary written")
}
