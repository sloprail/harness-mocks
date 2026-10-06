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
		"text":    {[]string{"--output-format", "text"}, "--output-format text is not implemented by the mock"},
		"json":    {[]string{"--output-format", "json"}, "--output-format json is not implemented by the mock"},
		"agent":   {[]string{"--agent", "reviewer"}, "--agent is not implemented by the mock"},
		"bare":    {[]string{"--bare"}, "--bare is not implemented by the mock"},
		"partial": {[]string{"--include-partial-messages"}, "--include-partial-messages is not implemented by the mock"},
		"input":   {[]string{"--input-format", "stream-json"}, "--input-format is not implemented by the mock"},
		"budget":  {[]string{"--max-budget-usd", "5"}, "--max-budget-usd is not implemented by the mock"},
		"rename":  {[]string{"--name", "n", "--continue"}, "--name with --resume or --continue is not implemented by the mock"},
	} {
		t.Run(name, func(t *testing.T) {
			out, code := e2etest.RunWithScript(t, script, append(tc.args, "--session-id", "s-1", "-p", "go")...)
			assert.NotZero(t, code, out)
			assert.Contains(t, out, tc.want)
			assert.NotContains(t, out, "RAN")
		})
	}
}

// A piped stdin, which the real run reads as input, is refused by name and the
// script never runs.
// sr:proves noninteractive-run/claude
func TestT001_09_PipedStdinIsRefused(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "s.sh")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\necho RAN\n"), 0o755))
	cmd := exec.Command(e2etest.MockBinaryPath, "--script", script, "--session-id", "s-1", "-p", "go")
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader("piped input\n")
	out, err := cmd.CombinedOutput()
	assert.Error(t, err)
	assert.Contains(t, string(out), "a piped stdin is not implemented by the mock")
	assert.NotContains(t, string(out), "RAN")
}
