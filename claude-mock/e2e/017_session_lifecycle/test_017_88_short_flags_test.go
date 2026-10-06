package e2e

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The CLI reference gives the short forms -r for --resume and -c for
// --continue: each resumes the earlier session exactly as the long form does
// (runs/resume-continue, forkresume).
// sr:proves session-resume/claude
func TestT017_88_ResumeAndContinueHaveShortForms(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"-r", []string{"-r", "earlier-1"}},
		{"-c", []string{"-c"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			cfg, log := filepath.Join(dir, "config"), filepath.Join(dir, "p.log")
			settings(t, dir, map[string]string{"SessionStart": payloadLogger(t, dir, "log.sh", log, "")})
			out, code := runInDir(t, dir, nil, "--script", script(t, dir, "a"), "--session-id", "earlier-1",
				"--project-dir", dir, "--config-dir", cfg, "-p", "one")
			require.Equal(t, 0, code, out)
			out, code = runInDir(t, dir, nil, append([]string{"--script", script(t, dir, "b"),
				"--project-dir", dir, "--config-dir", cfg, "-p", "two"}, tc.args...)...)
			require.Equal(t, 0, code, out)
			ps := payloads(t, log)
			require.Len(t, ps, 2)
			assert.Equal(t, "resume", ps[1]["source"])
			assert.Equal(t, "earlier-1", ps[1]["session_id"])
		})
	}
}
