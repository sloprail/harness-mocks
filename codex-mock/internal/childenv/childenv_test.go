package childenv

import (
	"slices"
	"testing"
)

// Recorded (runs/subprocess-session-env, runs/nested-session-env): a shell
// command sees CODEX_THREAD_ID and CODEX_SESSION_ID as the session's id, and
// CODEX_VERSION; launched inside another session it still sees this run's
// session id, while CODEX_CI passes through as inherited (a decoy reached the
// command unchanged) and is 1 when nothing was inherited.
// sr:proves subprocess-session-env/codex
func TestToolEnvIsThisRunsIdentity(t *testing.T) {
	bare := ToolEnv([]string{"PATH=/bin"}, "sess-1")
	for _, want := range []string{"CODEX_THREAD_ID=sess-1", "CODEX_SESSION_ID=sess-1", "CODEX_VERSION=" + Version, "CODEX_CI=1", "PATH=/bin"} {
		if !slices.Contains(bare, want) {
			t.Errorf("bare launch: %q missing from %v", want, bare)
		}
	}
	nested := ToolEnv([]string{"CODEX_THREAD_ID=decoy-outer", "CODEX_SESSION_ID=decoy", "CODEX_CI=decoy", "CODEX_VERSION=old"}, "sess-2")
	for _, want := range []string{"CODEX_THREAD_ID=sess-2", "CODEX_SESSION_ID=sess-2", "CODEX_VERSION=" + Version, "CODEX_CI=decoy"} {
		if !slices.Contains(nested, want) {
			t.Errorf("nested launch: %q missing from %v", want, nested)
		}
	}
	for _, kv := range nested {
		if kv == "CODEX_THREAD_ID=decoy-outer" || kv == "CODEX_SESSION_ID=decoy" {
			t.Errorf("an outer session id reached the command: %s", kv)
		}
	}
}

// Recorded: a hook command is handed no session variable; what it inherited
// reaches it as it was.
// sr:proves subprocess-session-env/codex
func TestHookIdentityAddsNothing(t *testing.T) {
	if len(HookIdentity()) != 0 {
		t.Fatalf("HookIdentity = %v, want none", HookIdentity())
	}
}
