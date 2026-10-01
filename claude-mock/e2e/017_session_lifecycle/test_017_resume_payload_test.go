package e2e

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestT017_31_ResumeStartPayloadCarriesTheResumeFacts: the SessionStart of a resume
// (and of a fork, which resumes the source) carries seconds_since_last_response,
// context_tokens, prompt_cache_likely_expired and estimated_cache_write_usd, as the
// recorded forkresume and compact runs show (hooks#sessionstart-input); a fresh
// session's does not. The mock spends no tokens, so the token facts are 0.
// sr:proves session-resume/claude
func TestT017_31_ResumeStartPayloadCarriesTheResumeFacts(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	log := filepath.Join(dir, "payloads.log")
	settings(t, dir, map[string]string{"SessionStart": payloadLogger(t, dir, "log.sh", log, "")})
	for _, args := range [][]string{
		{"--session-id", "rp-1"},
		{"--resume", "rp-1"},
		{"--resume", "rp-1", "--fork-session", "--session-id", "rp-2"},
	} {
		out, code := runInDir(t, dir, nil, append([]string{"--script", script(t, dir, "s"), "--project-dir", dir, "--config-dir", cfg, "-p", "go"}, args...)...)
		require.Equal(t, 0, code, out)
	}
	ps := payloads(t, log)
	require.Len(t, ps, 3)
	assert.Equal(t, "startup", ps[0]["source"])
	for _, k := range []string{"seconds_since_last_response", "context_tokens", "prompt_cache_likely_expired", "estimated_cache_write_usd"} {
		assert.NotContains(t, ps[0], k, "a fresh session has no resume facts")
	}
	for i, src := range map[int]string{1: "resume", 2: "fork"} {
		p := ps[i]
		assert.Equal(t, src, p["source"])
		assert.GreaterOrEqual(t, p["seconds_since_last_response"], float64(0))
		assert.Less(t, p["seconds_since_last_response"], float64(60), "just written")
		assert.Equal(t, float64(0), p["context_tokens"])
		assert.Equal(t, false, p["prompt_cache_likely_expired"])
		assert.Equal(t, float64(0), p["estimated_cache_write_usd"])
	}
}
