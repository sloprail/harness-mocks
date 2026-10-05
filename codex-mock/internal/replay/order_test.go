package replay

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	core "github.com/sloprail/harness-mocks/internal/replay"
)

// pendingCrossEventOrder is true while the codex adapter still sorts the hook
// payloads across the whole run (harness-mocks #207 item 3), which erases the
// order of different events. While it is true the test below expects that
// failure; once the adapter keeps cross-event order the test fails until the
// flag is removed, and from then on asserts the order for good.
const pendingCrossEventOrder = true

// stubMock is a "mock" that prints the recorded event stream and logs the hook
// payloads it is given, in the order given.
func stubMock(t *testing.T, stream string, payloads ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "mock.sh")
	script := "#!/bin/sh\ncat <<'EOF'\n" + stream + "\nEOF\n"
	for _, p := range payloads {
		script += "printf '%s\\n' '" + p + "' >> \"$HOOK_LOG\"\n"
	}
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// replayDiff replays a recording whose hook payloads are recorded in the order
// given, against a mock that logs them in the order mockOrder says; it returns
// what the replay reports as differing in the hook payloads.
func replayDiff(t *testing.T, recorded []string, mockOrder []string) string {
	t.Helper()
	const stream = `{"type":"thread.started","thread_id":"t"}`
	dir := t.TempDir()
	sample := filepath.Join(dir, "samples", "1")
	if err := os.MkdirAll(sample, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"stream.jsonl": stream + "\n", "payloads.jsonl": strings.Join(recorded, "\n") + "\n"} {
		if err := os.WriteFile(filepath.Join(sample, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	rec := core.Recording{Dir: dir, Prompt: "p", Setup: map[string]string{}, Agent: core.Agent{Final: "done"}}
	want, got, err := Adapter{Environ: os.Environ()}.Replay(stubMock(t, stream, mockOrder...), rec)
	if err != nil {
		t.Fatal(err)
	}
	return core.Diff("hook payloads", want.Hooks, got.Hooks)
}

// Two different events recorded in one order, and a mock that fires them in the
// other, are a difference the replay reports: sorting the payloads must not
// make them equal. The same payloads in the recorded order are no difference.
// sr:proves replay-fidelity
func TestHookPayloadSortKeepsOrderAcrossEvents(t *testing.T) {
	pre := `{"hook_event_name":"PreToolUse","tool_name":"Bash"}`
	post := `{"hook_event_name":"PostToolUse","tool_name":"Bash"}`
	if d := replayDiff(t, []string{pre, post}, []string{pre, post}); d != "" {
		t.Fatalf("the same events in the same order differ: %s", d)
	}
	swapped := replayDiff(t, []string{pre, post}, []string{post, pre})
	if pendingCrossEventOrder {
		if swapped != "" {
			t.Fatal("the adapter now keeps the order of different events: remove pendingCrossEventOrder")
		}
		t.Skip("pending #207 item 3: hook payloads are still sorted across events")
	}
	if swapped == "" {
		t.Fatal("a mock that fires PostToolUse before PreToolUse replays green against a recording that fires them the other way round")
	}
}
