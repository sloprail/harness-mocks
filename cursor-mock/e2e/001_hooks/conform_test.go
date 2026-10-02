package e2e

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// conforms asserts the mock's run showed what the recording showed: the same
// tool-call frames, the same exit statuses from the hook scripts, and the same
// payload to every hook, in the same order (apart from what the recordings'
// normalizer drops).
func conforms(t *testing.T, got, want observed) {
	t.Helper()
	require.Equal(t, want.frames, got.frames, "tool-call frames")
	require.Equal(t, want.results, got.results, "what the hook scripts decided")
	require.Equal(t, len(want.hooks), len(got.hooks), "number of hook payloads: got %v, want %v", hookNames(got), hookNames(want))
	for i := range want.hooks {
		require.Equal(t, want.hooks[i], got.hooks[i], "hook payload %d", i)
	}
}

func hookNames(o observed) (names []string) {
	for _, h := range o.hooks {
		names = append(names, h["hook_event_name"].(string))
	}
	return names
}

// eventsOf are the hook events a command raised, in order: the preToolUse
// naming it, and what followed up to the next preToolUse.
func eventsOf(o observed, command string) []string {
	var names []string
	in := false
	for _, h := range o.hooks {
		ev := h["hook_event_name"].(string)
		if ev == "preToolUse" {
			in = commandOf(h) == command
		}
		if ev == "sessionEnd" {
			in = false
		}
		if in {
			names = append(names, ev)
		}
	}
	return names
}

func commandOf(h map[string]any) string {
	if in, ok := h["tool_input"].(map[string]any); ok {
		c, _ := in["command"].(string)
		return c
	}
	return ""
}

// failureOf is the postToolUseFailure a command raised: its error_message and
// failure_type, and whether there was one.
func failureOf(o observed, command string) (message, kind string, ok bool) {
	for _, h := range o.hooks {
		if h["hook_event_name"] == "postToolUseFailure" && commandOf(h) == command {
			message, _ = h["error_message"].(string)
			kind, _ = h["failure_type"].(string)
			return message, kind, true
		}
	}
	return "", "", false
}

func joined(names []string) string { return strings.Join(names, " ") }
