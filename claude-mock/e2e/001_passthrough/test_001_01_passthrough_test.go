package e2e

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestT001_01_ScriptJSONLPassthrough verifies that JSONL lines emitted by the
// script are written to stdout verbatim and in order.
// staged:proves noninteractive-run/claude
func TestT001_01_ScriptJSONLPassthrough(t *testing.T) {
	script := `#!/bin/sh
printf '%s\n' '{"type":"system","subtype":"init","session_id":"test-session-001","tools":[]}'
printf '%s\n' '{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Hello from mock"}],"stop_reason":"end_turn"}}'
printf '%s\n' '{"type":"result","subtype":"success","result":"Hello from mock","is_error":false}'
`
	out, code := runWithScript(t, script,
		"--session-id", "test-session-001",
		"--output-format", "stream-json",
		"-p", "do the thing",
	)
	require.Equal(t, 0, code, "exit code should be 0; output:\n%s", out)

	lines := nonEmptyLines(out)
	require.GreaterOrEqual(t, len(lines), 3, "expected at least 3 JSONL lines")

	var sys map[string]any
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &sys))
	assert.Equal(t, "system", sys["type"])

	var asst map[string]any
	require.NoError(t, json.Unmarshal([]byte(lines[1]), &asst))
	assert.Equal(t, "assistant", asst["type"])

	var result map[string]any
	require.NoError(t, json.Unmarshal([]byte(lines[2]), &result))
	assert.Equal(t, "result", result["type"])
	assert.Equal(t, "Hello from mock", result["result"])
}

// TestT001_02_InvalidJSONLExitsNonZero verifies that a script emitting invalid
// JSON causes the mock to exit non-zero and print a diagnostic.
func TestT001_02_InvalidJSONLExitsNonZero(t *testing.T) {
	script := `#!/bin/sh
printf '%s\n' 'this is not json'
`
	out, code := runWithScript(t, script,
		"--session-id", "bad-session",
		"-p", "fail",
	)
	assert.NotEqual(t, 0, code, "should exit non-zero on invalid JSONL")
	assert.Contains(t, out, "invalid JSONL", "should mention invalid JSONL in output")
}

// TestT001_03_MissingTypeFieldExitsNonZero verifies that JSON missing the
// required "type" field is rejected.
func TestT001_03_MissingTypeFieldExitsNonZero(t *testing.T) {
	script := `#!/bin/sh
printf '%s\n' '{"subtype":"init","session_id":"x"}'
`
	out, code := runWithScript(t, script,
		"--session-id", "missing-type",
		"-p", "fail",
	)
	assert.NotEqual(t, 0, code)
	assert.Contains(t, out, "missing required field")
}

// TestT001_04_UnknownTypeExitsNonZero verifies that an unknown record type is
// rejected with a clear error message.
func TestT001_04_UnknownTypeExitsNonZero(t *testing.T) {
	script := `#!/bin/sh
printf '%s\n' '{"type":"bogus","data":"stuff"}'
`
	out, code := runWithScript(t, script,
		"--session-id", "bad-type",
		"-p", "fail",
	)
	assert.NotEqual(t, 0, code)
	assert.Contains(t, out, "unknown record type")
}

// TestT001_05_ResumeProbeExitsOneWhenNoResume verifies the A10N_MOCK_NO_RESUME
// behaviour: --resume with A10N_MOCK_NO_RESUME=1 exits 1 with an error-result
// frame, exactly as the real Claude Code CLI does for a fresh session.
func TestT001_05_ResumeProbeExitsOneWhenNoResume(t *testing.T) {
	script := writeScript(t, `#!/bin/sh
printf '%s\n' '{"type":"result","subtype":"success","result":"should not reach here","is_error":false}'
`)
	_ = script // won't be reached

	out, code := run(t,
		"--resume", "no-such-session",
		"-p", "do work",
		"A10N_MOCK_NO_RESUME=1", // not a flag — passed via env; test ignores this.
	)
	// NOTE: env vars can't be CLI args; use RunInDir with env instead.
	_ = out
	_ = code
	// Covered by the lower-level unit test; here we document the expected shape.
	t.Skip("use RunInDir with env={'A10N_MOCK_NO_RESUME=1'} for full coverage")
}

// TestT001_06_ResumeProbeEnv is the actual env-driven resume-probe test.
// sr:proves no-resume
func TestT001_06_ResumeProbeEnv(t *testing.T) {
	script := writeScript(t, `#!/bin/sh
printf '%s\n' '{"type":"result","subtype":"success","result":"should not reach here","is_error":false}'
`)

	cmd := strings.Join([]string{
		"--resume", "no-such-session-xyz",
		"--script", script,
		"-p", "do work",
	}, " ")
	_ = cmd

	dir := t.TempDir()
	out, code := runInDirWithEnv(t, dir,
		[]string{"A10N_MOCK_NO_RESUME=1"},
		"--resume", "no-such-session-xyz",
		"--script", script,
		"-p", "do work",
	)
	require.Equal(t, 1, code, "resume probe must exit 1; output: %s", out)
	assert.Contains(t, out, `"is_error":true`)
	assert.Contains(t, out, "no-such-session-xyz")
}

func nonEmptyLines(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) != "" {
			out = append(out, line)
		}
	}
	return out
}
