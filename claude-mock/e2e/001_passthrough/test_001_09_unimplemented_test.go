package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sloprail/harness-mocks/claude-mock/e2etest"
)

// The mock fails fast on what it does not implement (adr/fail-fast-unimplemented):
// an output format other than stream-json, --agent (which would put
// agent_type on main-thread hook payloads) and --name that renames a resumed session, are refused with an error naming
// them, and the script never runs.
// sr:proves noninteractive-run/claude
// sr:proves hook-common-payload/claude
func TestT001_09_UnimplementedInputsAreRefused(t *testing.T) {
	const script = "#!/bin/sh\necho RAN\n"
	for name, tc := range map[string]struct {
		args []string
		want string
	}{
		"text":     {[]string{"--output-format", "text"}, "--output-format text is not implemented by the mock"},
		"json":     {[]string{"--output-format", "json"}, "--output-format json is not implemented by the mock"},
		"agent":    {[]string{"--agent", "reviewer"}, "--agent is not implemented by the mock"},
		"bare":     {[]string{"--bare"}, "--bare is not implemented by the mock"},
		"partial":  {[]string{"--include-partial-messages"}, "--include-partial-messages is not implemented by the mock"},
		"input":    {[]string{"--input-format", "stream-json"}, "--input-format is not implemented by the mock"},
		"budget":   {[]string{"--max-budget-usd", "5"}, "--max-budget-usd is not implemented by the mock"},
		"schema":   {[]string{"--json-schema", "{}"}, "unknown option '--json-schema'"},
		"fallback": {[]string{"--fallback-model", "sonnet"}, "unknown option '--fallback-model'"},
		"rename":   {[]string{"--name", "n", "--continue"}, "--name with --resume or --continue is not implemented by the mock"},
	} {
		t.Run(name, func(t *testing.T) {
			out, code := e2etest.RunWithScript(t, script, append(tc.args, "--session-id", "s-1", "-p", "go")...)
			assert.NotZero(t, code, out)
			assert.Contains(t, out, tc.want)
			assert.NotContains(t, out, "RAN")
		})
	}
}

// A piped stdin beside a prompt argument, which the real run combines, is refused by
// name and the script never runs.
// sr:proves noninteractive-run/claude
func TestT001_09_PipedStdinWithPromptArgIsRefused(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "s.sh")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\necho RAN\n"), 0o755))
	cmd := exec.Command(e2etest.MockBinaryPath, "--script", script, "--session-id", "s-1", "-p", "go")
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader("piped input\n")
	out, err := cmd.CombinedOutput()
	assert.Error(t, err)
	assert.Contains(t, string(out), "a piped stdin together with a prompt argument is not implemented by the mock")
	assert.NotContains(t, string(out), "RAN")
}

// `claude -p` given no prompt argument reads the prompt from a piped stdin (the way
// sr-agent hands a large prompt over): the script sees it as A10N_MOCK_PROMPT.
// sr:proves noninteractive-run/claude
func TestT001_09_PipedStdinIsThePrompt(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "s.sh")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\nprintf '%s' \"$A10N_MOCK_PROMPT\" > prompt.txt\n"), 0o755))
	cmd := exec.Command(e2etest.MockBinaryPath, "--script", script, "--session-id", "s-1", "-p", "--output-format", "stream-json", "--verbose")
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader("line one\nline two\n")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	got, err := os.ReadFile(filepath.Join(dir, "prompt.txt"))
	require.NoError(t, err)
	assert.Equal(t, "line one\nline two", string(got))
}
