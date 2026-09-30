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

// These tests pin the RESUME-persists-the-prompt behaviour end-to-end through the
// compiled mock: a fresh run (--session-id) opens the transcript with the prompt as
// the parentless ROOT, and a resume run (--resume <same id>) against that existing
// transcript APPENDS its prompt as a continuation human record — its own uuid and a
// NON-null parentUuid — after the prior cycle's records, without rewriting the root.
//
// This is what lets a test build a genuine multi-human-turn transcript by running the
// mock twice on one session id, rather than co-emitting two records in one cycle.

// resultScript is a minimal scenario: emit one result frame and end the turn. It
// carries no tool_use, so a fresh run and a resume run produce the same trajectory
// shape (root/continuation prompt + result), which is all these tests read.
const resultScript = `#!/bin/sh
printf '%s\n' '{"type":"result","subtype":"success","result":"done","is_error":false}'
`

// transcriptRec is the subset of a transcript record these tests assert on.
type transcriptRec struct {
	Type       string          `json:"type"`
	UUID       string          `json:"uuid"`
	ParentUUID *string         `json:"parentUuid"`
	SessionID  string          `json:"sessionId"`
	Message    json.RawMessage `json:"message"`
}

// userContent returns the plain string content of a user record's message, or ""
// when the content is not a bare string (a tool_result block list, etc.).
func (r transcriptRec) userContent() string {
	var msg struct {
		Content json.RawMessage `json:"content"`
	}
	if json.Unmarshal(r.Message, &msg) != nil {
		return ""
	}
	var s string
	if json.Unmarshal(msg.Content, &s) != nil {
		return ""
	}
	return s
}

// readTranscript reads every JSONL record of the single session file under configDir.
func readTranscript(t *testing.T, configDir string) []transcriptRec {
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

	var recs []transcriptRec
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var rec transcriptRec
		require.NoError(t, json.Unmarshal([]byte(line), &rec), "line: %s", line)
		recs = append(recs, rec)
	}
	return recs
}

// userRecords returns the transcript's user records, in order.
func userRecords(recs []transcriptRec) []transcriptRec {
	var out []transcriptRec
	for _, r := range recs {
		if r.Type == "user" {
			out = append(out, r)
		}
	}
	return out
}

// TestT015_01_FreshRunSeedsParentlessRoot: a FRESH run (--session-id) writes the
// prompt as the transcript's first user record — a parentless ROOT (unchanged
// behaviour, the baseline the resume path is measured against).
func TestT015_01_FreshRunSeedsParentlessRoot(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, "config")
	script := filepath.Join(dir, "s.sh")
	require.NoError(t, os.WriteFile(script, []byte(resultScript), 0o755))

	_, code := runInDir(t, dir, nil,
		"--script", script, "--session-id", "s-fresh", "--project-dir", dir,
		"--config-dir", configDir, "-p", "the ROOT human prompt")
	require.Equal(t, 0, code)

	users := userRecords(readTranscript(t, configDir))
	require.NotEmpty(t, users, "the fresh run must seed a user record")
	root := users[0]
	assert.Equal(t, "the ROOT human prompt", root.userContent(), "the prompt is the first user record")
	// A parentless root: parentUuid is null/absent, not a chained value.
	assert.Nil(t, root.ParentUUID, "the fresh root must be parentless (null/absent parentUuid)")
}

// TestT015_02_ResumeAppendsContinuationHumanRecord: a RESUME run against an existing
// transcript APPENDS its prompt as a user record with a NON-null parentUuid, AFTER the
// prior cycle's records — a mid-conversation human turn, not a second root.
// sr:proves session-resume/claude
func TestT015_02_ResumeAppendsContinuationHumanRecord(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, "config")
	script := filepath.Join(dir, "s.sh")
	require.NoError(t, os.WriteFile(script, []byte(resultScript), 0o755))

	// Run 1: fresh — seeds the root.
	_, code := runInDir(t, dir, nil,
		"--script", script, "--session-id", "s-cont", "--project-dir", dir,
		"--config-dir", configDir, "-p", "first human turn")
	require.Equal(t, 0, code)

	// Run 2: resume the SAME session id — appends its prompt as a continuation.
	_, code = runInDir(t, dir, nil,
		"--script", script, "--resume", "s-cont", "--project-dir", dir,
		"--config-dir", configDir, "-p", "second human turn")
	require.Equal(t, 0, code)

	recs := readTranscript(t, configDir)
	users := userRecords(recs)
	require.Len(t, users, 2, "two human records: the seeded root and the appended continuation")

	root, cont := users[0], users[1]
	assert.Equal(t, "first human turn", root.userContent())
	assert.Nil(t, root.ParentUUID, "the root must stay parentless")

	assert.Equal(t, "second human turn", cont.userContent(), "the resume prompt landed as a human turn")
	assert.Regexp(t, `^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`, cont.UUID,
		"the continuation must carry a fresh v4 uuid, or a transcript reader skips it as a no-uuid line")
	require.NotNil(t, cont.ParentUUID, "the continuation's parentUuid must be present and NON-null")
	assert.NotEmpty(t, *cont.ParentUUID, "a non-null parent is what keeps it from reading as a second root")

	// The continuation sits AFTER the root physically (its record index is greater).
	rootIdx, contIdx := -1, -1
	for i, r := range recs {
		switch r.userContent() {
		case "first human turn":
			rootIdx = i
		case "second human turn":
			contIdx = i
		}
	}
	assert.Greater(t, contIdx, rootIdx, "the continuation must be appended after the prior cycle's records")
}

// TestT015_03_TwoResumesTwoDistinctHumanRecords: two resume runs on one session id
// produce TWO distinct continuation human records, each with its own uuid — the
// multi-human-turn transcript two same-session runs are meant to build.
// sr:proves session-resume/claude
func TestT015_03_TwoResumesTwoDistinctHumanRecords(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, "config")
	script := filepath.Join(dir, "s.sh")
	require.NoError(t, os.WriteFile(script, []byte(resultScript), 0o755))

	_, code := runInDir(t, dir, nil,
		"--script", script, "--session-id", "s-multi", "--project-dir", dir,
		"--config-dir", configDir, "-p", "human one")
	require.Equal(t, 0, code)
	_, code = runInDir(t, dir, nil,
		"--script", script, "--resume", "s-multi", "--project-dir", dir,
		"--config-dir", configDir, "-p", "human two")
	require.Equal(t, 0, code)
	_, code = runInDir(t, dir, nil,
		"--script", script, "--resume", "s-multi", "--project-dir", dir,
		"--config-dir", configDir, "-p", "human three")
	require.Equal(t, 0, code)

	users := userRecords(readTranscript(t, configDir))
	require.Len(t, users, 3, "root + two distinct continuations")
	assert.Equal(t, "human one", users[0].userContent())
	assert.Equal(t, "human two", users[1].userContent())
	assert.Equal(t, "human three", users[2].userContent())

	assert.NotEmpty(t, users[1].UUID)
	assert.NotEmpty(t, users[2].UUID)
	assert.NotEqual(t, users[1].UUID, users[2].UUID, "each continuation carries its own uuid")
	require.NotNil(t, users[1].ParentUUID)
	require.NotNil(t, users[2].ParentUUID)
}

// TestT015_04_ResumeDoesNotRewriteRoot: a resume must not duplicate or rewrite the
// root — the seeded first prompt appears EXACTLY once after a resume cycle.
// sr:proves session-resume/claude
func TestT015_04_ResumeDoesNotRewriteRoot(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, "config")
	script := filepath.Join(dir, "s.sh")
	require.NoError(t, os.WriteFile(script, []byte(resultScript), 0o755))

	_, code := runInDir(t, dir, nil,
		"--script", script, "--session-id", "s-norewrite", "--project-dir", dir,
		"--config-dir", configDir, "-p", "the one and only root")
	require.Equal(t, 0, code)
	_, code = runInDir(t, dir, nil,
		"--script", script, "--resume", "s-norewrite", "--project-dir", dir,
		"--config-dir", configDir, "-p", "a distinct follow-up")
	require.Equal(t, 0, code)

	recs := readTranscript(t, configDir)
	rootCount := 0
	for _, r := range recs {
		if r.userContent() == "the one and only root" {
			rootCount++
		}
	}
	assert.Equal(t, 1, rootCount, "the root prompt must appear exactly once — the resume must not rewrite it")
}
