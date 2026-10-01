package e2e

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestT009_18_SubagentHookMatchersSelectTheAgentType: the matcher of SubagentStart
// and SubagentStop is the agent type (hooks#subagentstart, hooks#subagentstop): a
// hook for "Explore" does not fire for a general-purpose sub-agent, one for
// "general-purpose" fires on both events.
// sr:proves subagent-lifecycle-hooks/claude
func TestT009_18_SubagentHookMatchersSelectTheAgentType(t *testing.T) {
	for _, tc := range []struct {
		matcher string
		fires   bool
	}{{"general-purpose", true}, {"Explore", false}} {
		t.Run(tc.matcher, func(t *testing.T) {
			dir := t.TempDir()
			log := filepath.Join(dir, "hook.log")
			hook := writeHook(t, dir, "hook.sh", `cat >/dev/null; echo fired >> `+log)
			entry := `[{"matcher":"` + tc.matcher + `","hooks":[{"type":"command","command":"` + hook + `"}]}]`
			require.NoError(t, os.MkdirAll(filepath.Join(dir, ".claude"), 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(dir, ".claude", "settings.json"),
				[]byte(`{"hooks":{"SubagentStart":`+entry+`,"SubagentStop":`+entry+`}}`), 0o644))
			sub := writeScript(t, dir, "sub.sh", "#!/bin/sh\nprintf '%s\\n' '{\"type\":\"result\",\"subtype\":\"success\",\"result\":\"sub done\",\"is_error\":false}'\n")
			out, code := runInDir(t, dir, nil, "--script", writeScript(t, dir, "orch.sh", orchestratorScript("Agent", sub)),
				"--session-id", "sub-matcher", "--project-dir", dir, "-p", "go")
			require.Equal(t, 0, code, out)
			data, err := os.ReadFile(log)
			if !tc.fires {
				assert.True(t, os.IsNotExist(err), "the %s matcher does not select a general-purpose sub-agent", tc.matcher)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, "fired\nfired\n", string(data), "start and stop")
		})
	}
}
