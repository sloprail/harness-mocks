package e2e

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A PostToolUseFailure hook is selected by the failed tool's name, as the
// other tool events are (docs, Matcher patterns): a matcher naming Bash fires
// for a failing Bash call, one naming Read does not (runs/bashfail is the
// failing call).
// sr:proves hook-matcher-filter/claude
func TestT017_93_PostToolUseFailureMatchesTheToolsName(t *testing.T) {
	for matcher, fires := range map[string]bool{"Bash": true, "Read": false} {
		t.Run(matcher, func(t *testing.T) {
			dir := t.TempDir()
			cfg, log := filepath.Join(dir, "config"), filepath.Join(dir, "p.log")
			compactSettings(t, dir, map[string][2]string{"PostToolUseFailure": {matcher, payloadLogger(t, dir, "log.sh", log, "")}})
			sc := script(t, dir, "s", toolUse("f1", "Bash", `{"command":"exit 3"}`))
			out, code := runInDir(t, dir, nil, "--script", sc, "--session-id", "pf-"+matcher, "--project-dir", dir, "--config-dir", cfg, "-p", "go")
			require.Equal(t, 0, code, out)
			if !fires {
				assert.NoFileExists(t, log)
				return
			}
			ps := payloads(t, log)
			require.Len(t, ps, 1)
			assert.Equal(t, "Bash", ps[0]["tool_name"])
		})
	}
}
