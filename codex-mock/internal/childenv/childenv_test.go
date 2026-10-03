package childenv

import (
	"slices"
	"testing"

	"github.com/sloprail/harness-mocks/internal/procexec"
)

// Recorded (runs/subprocess-session-env, runs/nested-session-env): a shell
// command sees CODEX_THREAD_ID and CODEX_SESSION_ID as the session's id, and
// CODEX_VERSION; launched inside another session it still sees this run's
// session id, while CODEX_CI passes through as inherited (a decoy reached the
// command unchanged) and is 1 when nothing was inherited.
// sr:proves subprocess-session-env/codex
func TestToolEnvIsThisRunsIdentity(t *testing.T) {
	bare := ToolEnv([]string{"PATH=/bin"}, "sess-1")
	for _, want := range []string{"CODEX_THREAD_ID=sess-1", "CODEX_SESSION_ID=sess-1", "CODEX_VERSION=" + Version, "CODEX_CI=1", "CODEX_MANAGED_BY_NPM=1", "PATH=/bin"} {
		if !slices.Contains(bare, want) {
			t.Errorf("bare launch: %q missing from %v", want, bare)
		}
	}
	nested := ToolEnv([]string{"CODEX_THREAD_ID=decoy-outer", "CODEX_SESSION_ID=decoy", "CODEX_CI=decoy", "CODEX_VERSION=old", "CODEX_MANAGED_BY_NPM=decoy"}, "sess-2")
	for _, want := range []string{"CODEX_THREAD_ID=sess-2", "CODEX_SESSION_ID=sess-2", "CODEX_VERSION=" + Version, "CODEX_CI=decoy", "CODEX_MANAGED_BY_NPM=1"} {
		if !slices.Contains(nested, want) {
			t.Errorf("nested launch: %q missing from %v", want, nested)
		}
	}
	for _, kv := range nested {
		if kv == "CODEX_THREAD_ID=decoy-outer" || kv == "CODEX_SESSION_ID=decoy" || kv == "CODEX_MANAGED_BY_NPM=decoy" {
			t.Errorf("an outer session id reached the command: %s", kv)
		}
	}
}

// Recorded: a hook command is handed the launcher's variables (the npm
// launcher's CODEX_MANAGED_BY_NPM=1, replacing a decoy, and the package root)
// and no session variable; what else it inherited reaches it as it was.
// sr:proves subprocess-session-env/codex
func TestHookIdentityIsTheLauncherOnly(t *testing.T) {
	id := HookIdentity()
	if id["CODEX_MANAGED_BY_NPM"] != "1" || id["CODEX_MANAGED_PACKAGE_ROOT"] == "" || len(id) != 2 {
		t.Fatalf("HookIdentity = %v, want only the launcher's variables", id)
	}
	env := procexec.Env([]string{"CODEX_MANAGED_BY_NPM=decoy", "CODEX_THREAD_ID=outer"}, id, nil)
	for _, want := range []string{"CODEX_MANAGED_BY_NPM=1", "CODEX_THREAD_ID=outer"} {
		if !slices.Contains(env, want) {
			t.Errorf("%q missing from %v", want, env)
		}
	}
	if slices.Contains(env, "CODEX_MANAGED_BY_NPM=decoy") {
		t.Errorf("a decoy reached the hook: %v", env)
	}
}
