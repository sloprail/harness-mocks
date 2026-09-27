package toolexec

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBashExportsSessionIDOverInheritedValue: a Bash tool subprocess sees the
// mock's session id as CLAUDE_CODE_SESSION_ID even when the mock's own process
// environment carries a DIFFERENT one (e.g. the operator's outer Claude Code
// session, when tests run inside a live session). Real Claude Code exports the
// active session id into every Bash tool subprocess.
// sr:docs https://code.claude.com/docs/en/env-vars (CLAUDE_CODE_SESSION_ID)
func TestBashExportsSessionIDOverInheritedValue(t *testing.T) {
	t.Setenv("CLAUDE_CODE_SESSION_ID", "decoy-outer-session")

	res := Execute(context.Background(), "Bash",
		[]byte(`{"command":"printf %s \"$CLAUDE_CODE_SESSION_ID\""}`), t.TempDir(), "mock-session-42")

	require.False(t, res.IsError, "bash failed: %s", res.Output)
	assert.Equal(t, "mock-session-42", res.Output)
}

// TestBashKeepsInheritedEnvironment: exporting the session id must not strip the
// rest of the mock's environment from the Bash subprocess.
func TestBashKeepsInheritedEnvironment(t *testing.T) {
	t.Setenv("TOOLEXEC_TEST_INHERITED", "still-here")

	res := Execute(context.Background(), "Bash",
		[]byte(`{"command":"printf %s \"$TOOLEXEC_TEST_INHERITED\""}`), t.TempDir(), "mock-session-42")

	require.False(t, res.IsError, "bash failed: %s", res.Output)
	assert.Equal(t, "still-here", res.Output)
}
