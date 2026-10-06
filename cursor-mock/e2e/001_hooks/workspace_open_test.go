package e2e

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWorkspaceOpenFiresFirstWithNoSession: recorded (runs/workspace-open),
// cursor-agent in print mode fires the workspaceOpen hook once, before the
// session's start hook, and its payload is not a session's: only the event's
// name, the Cursor version, the workspace roots and the user's email (null),
// with no conversation, session, model or transcript path. The mock fires it
// the same, with the same fields.
// sr:proves hook-common-payload/cursor
// sr:proves session-transcript-file/cursor
func TestWorkspaceOpenFiresFirstWithNoSession(t *testing.T) {
	got, want := replay(t, "workspace-open")
	conforms(t, got, want)

	for name, payloads := range map[string][]map[string]any{"recorded": recordedRaw(t, "workspace-open"), "mock": got.raw} {
		require.NotEmpty(t, payloads, name)
		assert.Equal(t, "workspaceOpen", payloads[0]["hook_event_name"], name+": it fires first")
		var keys []string
		for k := range payloads[0] {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		assert.Equal(t, []string{"cursor_version", "hook_event_name", "user_email", "workspace_roots"}, keys, name+": no session fields")
		assert.Nil(t, payloads[0]["user_email"], name)
		assert.Equal(t, "sessionStart", payloads[1]["hook_event_name"], name+": the session starts after it")
	}
}
