package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests pin the mock transcript FILE to the shape real Claude Code writes,
// verified against a real ~/.claude/projects/<proj>/<session>.jsonl:
//
//   - Every persisted record carries a uuid; the ROOT carries a uuid with a null
//     parentUuid (the origin), and every later record carries a NON-null parentUuid
//     that chains it to the record written before it (FIX 1).
//   - The `result` frame is STREAM-only: it appears on stdout but is NEVER persisted
//     to the transcript file — a real transcript holds zero type:"result" records
//     (FIX 2).
//   - The no-uuid preamble records a real session opens with (custom-title / mode /
//     last-prompt / …) are counted as physical lines but skipped as entries, so a
//     uuid-carrying entry's physical line runs PAST its entry ordinal.

var uuidV4Re = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

// fileRec is the subset of a transcript record these tests assert on.
type fileRec struct {
	Type       string          `json:"type"`
	UUID       string          `json:"uuid"`
	ParentUUID *string         `json:"parentUuid"`
	Message    json.RawMessage `json:"message"`
}

// readTranscriptLines reads the single session file under configDir and returns its
// non-empty JSONL lines, in physical order.
func readTranscriptLines(t *testing.T, configDir string) []string {
	t.Helper()
	var sessionFile string
	require.NoError(t, filepath.Walk(configDir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && filepath.Ext(p) == ".jsonl" {
			sessionFile = p
		}
		return nil
	}))
	require.NotEmpty(t, sessionFile, "a session transcript must exist under %s", configDir)
	data, err := os.ReadFile(sessionFile)
	require.NoError(t, err)
	var lines []string
	for _, l := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, l)
		}
	}
	return lines
}

func parseRecs(t *testing.T, lines []string) []fileRec {
	t.Helper()
	var recs []fileRec
	for _, l := range lines {
		var r fileRec
		require.NoError(t, json.Unmarshal([]byte(l), &r), "line: %s", l)
		recs = append(recs, r)
	}
	return recs
}

// hasToolResult reports whether a record's message carries a tool_result block.
func hasToolResult(r fileRec) bool {
	var msg struct {
		Content []struct {
			Type string `json:"type"`
		} `json:"content"`
	}
	if json.Unmarshal(r.Message, &msg) != nil {
		return false
	}
	for _, b := range msg.Content {
		if b.Type == "tool_result" {
			return true
		}
	}
	return false
}

// chainScript: one assistant tool_use turn (the mock executes it and synthesises a
// tool_result), then a final result frame once the tool_result is in history. This is
// the trajectory whose FILE shape the chain assertions read: root, assistant tool_use,
// synthesised tool_result — three chained records — and a result frame that is streamed
// but not persisted.
const chainScript = `#!/bin/sh
if [ -n "$A10N_MOCK_SESSION_FILE" ] && grep -q "tool_result" "$A10N_MOCK_SESSION_FILE" 2>/dev/null; then
  printf '%s\n' '{"type":"result","subtype":"success","result":"done","is_error":false}'
  exit 0
fi
printf '%s\n' '{"type":"assistant","message":{"role":"assistant","stop_reason":null,"content":[{"type":"tool_use","id":"tu_1","name":"Bash","input":{"command":"echo hi"}}]}}'
`

// TestT016_01_EveryRecordChainsWithAUUID: the persisted records form a real chain —
// each carries a uuid, the root is parentless, and every later record's parentUuid is
// the uuid of the record before it. This is the shape real Claude Code writes and the
// mock previously diverged from (its streamed records carried no uuid at all).
func TestT016_01_EveryRecordChainsWithAUUID(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, "config")
	script := filepath.Join(dir, "s.sh")
	require.NoError(t, os.WriteFile(script, []byte(chainScript), 0o755))

	stdout, code := runInDir(t, dir, nil,
		"--script", script, "--session-id", "s-chain", "--project-dir", dir,
		"--config-dir", configDir, "-p", "kick it off")
	require.Equal(t, 0, code)

	allRecs := parseRecs(t, readTranscriptLines(t, configDir))

	// Drop the no-uuid preamble head (custom-title / mode / last-prompt): those carry no
	// uuid and are not part of the chain. The chain assertions below are over the
	// uuid-carrying records — the root and every turn after it.
	var recs []fileRec
	for _, r := range allRecs {
		if r.UUID != "" {
			recs = append(recs, r)
		}
	}
	require.GreaterOrEqual(t, len(recs), 3,
		"root + assistant tool_use + synthesised tool_result must all be persisted")

	// The first uuid-carrying record is the origin: a uuid with an explicit null
	// parentUuid.
	root := recs[0]
	assert.Equal(t, "user", root.Type, "the first uuid-carrying record is the root user prompt")
	assert.Nil(t, root.ParentUUID, "the root must be parentless (null parentUuid) — the origin marker")

	// Every later record chains from the one before it: a NON-null parentUuid equal to
	// the previous record's uuid.
	for i := 1; i < len(recs); i++ {
		require.NotNil(t, recs[i].ParentUUID, "record %d (type %q) must have a non-null parentUuid", i, recs[i].Type)
		assert.Equal(t, recs[i-1].UUID, *recs[i].ParentUUID,
			"record %d must chain to the previous record's uuid", i)
	}

	// The synthesised tool_result specifically: it carried no uuid when emitted, so the
	// mock must have MINTED a v4 uuid for it and chained it — the FIX 1 core.
	var tr *fileRec
	for i := range recs {
		if hasToolResult(recs[i]) {
			tr = &recs[i]
			break
		}
	}
	require.NotNil(t, tr, "a synthesised tool_result record must be persisted")
	assert.Regexp(t, uuidV4Re, tr.UUID, "the synthesised tool_result must carry a minted v4 uuid")
	require.NotNil(t, tr.ParentUUID)
	assert.NotEmpty(t, *tr.ParentUUID, "the tool_result's parentUuid must be non-null")

	// FIX 2 restated here for this trajectory: the result frame is on stdout…
	assert.Contains(t, stdout, `"type":"result"`, "the result frame must stream to stdout")
	// …and absent from the file.
	for _, r := range recs {
		assert.NotEqual(t, "result", r.Type, "no result record may be persisted to the transcript file")
	}
}

// TestT016_02_ResultFrameIsStreamOnly: the result frame reaches stdout but is never
// written to the transcript file — a real transcript holds zero type:"result" records.
func TestT016_02_ResultFrameIsStreamOnly(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, "config")
	script := filepath.Join(dir, "s.sh")
	require.NoError(t, os.WriteFile(script, []byte(`#!/bin/sh
printf '%s\n' '{"type":"result","subtype":"success","result":"done","is_error":false}'
`), 0o755))

	stdout, code := runInDir(t, dir, nil,
		"--script", script, "--session-id", "s-resonly", "--project-dir", dir,
		"--config-dir", configDir, "-p", "go")
	require.Equal(t, 0, code)

	assert.Contains(t, stdout, `"type":"result"`, "the result frame is a stdout-stream frame")
	lines := readTranscriptLines(t, configDir)
	for _, l := range lines {
		assert.NotContains(t, l, `"type":"result"`,
			"real Claude Code never persists the result frame to the transcript file")
	}
	// The transcript still holds the seeded root prompt (the file is not empty).
	require.NotEmpty(t, lines, "the seeded root prompt is still persisted")
}

// TestT016_03_FreshTranscriptOpensWithNoUUIDPreamble: a FRESH mock session opens its
// transcript with the no-uuid preamble records a real Claude Code session file does —
// custom-title / mode / last-prompt at the HEAD, each carrying no uuid — BEFORE the root
// prompt record. This is a fidelity fix: the mock previously opened straight at the user
// prompt, whereas a real transcript opens with these bookkeeping records.
func TestT016_03_FreshTranscriptOpensWithNoUUIDPreamble(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, "config")
	script := filepath.Join(dir, "s.sh")
	require.NoError(t, os.WriteFile(script, []byte(chainScript), 0o755))

	_, code := runInDir(t, dir, nil,
		"--script", script, "--session-id", "s-preamble", "--project-dir", dir,
		"--config-dir", configDir, "-p", "start")
	require.Equal(t, 0, code)

	lines := readTranscriptLines(t, configDir)
	recs := parseRecs(t, lines)
	require.GreaterOrEqual(t, len(recs), 4, "preamble block + root + conversation")

	// The head is the preamble block: a contiguous run of no-uuid records ending before
	// the first uuid-carrying record (the root). At least one is a custom-title.
	preambleCount := 0
	sawCustomTitle := false
	for _, r := range recs {
		if r.UUID != "" {
			break // reached the root — preamble is over
		}
		assert.Nil(t, r.ParentUUID, "a preamble record carries no parentUuid")
		assert.True(t, preambleTypeSet[r.Type], "a head record must be a known preamble type, got %q", r.Type)
		if r.Type == "custom-title" {
			sawCustomTitle = true
		}
		preambleCount++
	}
	assert.GreaterOrEqual(t, preambleCount, 2, "the transcript opens with a preamble block")
	assert.True(t, sawCustomTitle, "the preamble includes a custom-title, as real Claude Code writes")

	// The FIRST uuid-carrying record is the root, and it sits on a physical line PAST
	// its entry ordinal (1) by the number of skipped preamble lines — the count-but-skip
	// behaviour a line-number derivation rests on.
	rootLine := 0
	for i, l := range lines {
		var r fileRec
		require.NoError(t, json.Unmarshal([]byte(l), &r))
		if r.UUID != "" {
			rootLine = i + 1
			break
		}
	}
	assert.Equal(t, preambleCount+1, rootLine,
		"the root's physical line is past its ordinal by the skipped preamble lines")
	assert.Greater(t, rootLine, 1, "a uuid-carrying entry's physical line runs past its ordinal")
}

// TestT016_04_ResumeDoesNotAddASecondPreamble: a resume run does NOT open with a fresh
// preamble block — the transcript already opened, and a real session's bookkeeping head
// is written once. So the file still has exactly one preamble block at the head after a
// resume cycle.
func TestT016_04_ResumeDoesNotAddASecondPreamble(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, "config")
	script := filepath.Join(dir, "s.sh")
	require.NoError(t, os.WriteFile(script, []byte(`#!/bin/sh
printf '%s\n' '{"type":"result","subtype":"success","result":"done","is_error":false}'
`), 0o755))

	_, code := runInDir(t, dir, nil,
		"--script", script, "--session-id", "s-once", "--project-dir", dir,
		"--config-dir", configDir, "-p", "first turn")
	require.Equal(t, 0, code)
	_, code = runInDir(t, dir, nil,
		"--script", script, "--resume", "s-once", "--project-dir", dir,
		"--config-dir", configDir, "-p", "second turn")
	require.Equal(t, 0, code)

	recs := parseRecs(t, readTranscriptLines(t, configDir))
	titles := 0
	for _, r := range recs {
		if r.Type == "custom-title" {
			titles++
		}
	}
	assert.Equal(t, 1, titles, "the preamble block (its custom-title) is written exactly once, not per-resume")
}

// preambleTypeSet mirrors the mock's own set of no-uuid preamble record types, for
// asserting a head record is one of them.
var preambleTypeSet = map[string]bool{
	"custom-title":    true,
	"ai-title":        true,
	"mode":            true,
	"last-prompt":     true,
	"queue-operation": true,
	"pr-link":         true,
}
