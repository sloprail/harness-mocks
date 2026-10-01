package hooks

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	corehooks "github.com/sloprail/harness-mocks/internal/hooks"
)

func out(exit int, stdout, stderr string) corehooks.Outcome {
	return corehooks.Outcome{Started: true, Exit: exit, Stdout: stdout, Stderr: stderr}
}

var plain = Entry{Command: "h.sh"}

// sr:proves hook-exit-code-semantics/cursor
func TestInterpretByExitStatusAndOutput(t *testing.T) {
	const invalid = `Hook "h.sh" returned invalid JSON. The command was blocked for safety.`
	for _, tc := range []struct {
		name  string
		event Event
		o     corehooks.Outcome
		want  Decision
	}{
		{"exit 2 blocks with stderr", BeforeShellExecution, out(2, "", "no\n"), Decision{Permission: "deny", Blocked: true, Message: "Hook blocked with message: no"}},
		{"exit 1 fails open, output unread", BeforeShellExecution, out(1, `{"permission":"deny"}`, ""), Decision{}},
		{"exit 3 fails open", PreToolUse, out(3, "", "x"), Decision{}},
		{"silence allows", BeforeShellExecution, out(0, "  \n", ""), Decision{}},
		{"json deny", PreToolUse, out(0, `{"permission":"deny","user_message":"why"}`, ""), Decision{Permission: "deny", Message: "why"}},
		{"json allow", PreToolUse, out(0, `{"permission":"allow"}`, ""), Decision{Permission: "allow"}},
		{"text blocks", BeforeShellExecution, out(0, "not json", ""), Decision{Permission: "deny", Message: invalid}},
		{"unknown permission blocks", PreToolUse, out(0, `{"permission":"maybe"}`, ""), Decision{Permission: "deny", Message: invalid}},
		{"a hook on another event is not read", PostToolUse, out(0, "not json", ""), Decision{}},
		{"a command that did not start fails open", PreToolUse, corehooks.Outcome{Exit: -1}, Decision{}},
	} {
		if got := Interpret(tc.event, plain, tc.o); got != tc.want {
			t.Errorf("%s: Interpret = %+v, want %+v", tc.name, got, tc.want)
		}
	}
}

// sr:proves hook-exit-code-semantics/cursor
func TestAFailClosedHookBlocksOnAnyFailure(t *testing.T) {
	h := Entry{Command: "h.sh", FailClosed: true}
	got := Interpret(BeforeShellExecution, h, out(3, "", "boom\n"))
	want := Decision{Permission: "deny", Blocked: true, Message: `Tool blocked because this hook is configured to fail closed (block when it fails). Hook "h.sh" failed with exit code 3: boom`}
	if got != want {
		t.Errorf("non-zero status: %+v, want %+v", got, want)
	}
	got = Interpret(PreToolUse, h, out(0, "", ""))
	want = Decision{Permission: "deny", Message: `Tool blocked because this hook is configured to fail closed (block when it fails). Hook "h.sh" returned no output.`}
	if got != want {
		t.Errorf("no output: %+v, want %+v", got, want)
	}
	if got := Interpret(AfterShellExecution, h, out(0, "", "")); got != (Decision{}) {
		t.Errorf("silence from an event with nothing to refuse: %+v", got)
	}
	if got := Interpret(BeforeShellExecution, h, out(2, "", "no")); got.Message != "Hook blocked with message: no" {
		t.Errorf("exit 2 keeps its own message: %+v", got)
	}
}

// sr:proves pretooluse-refusal/cursor
func TestRefusalDenyWinsAndABlockOutranksIt(t *testing.T) {
	deny := Decision{Permission: "deny", Message: "json says no"}
	block := Decision{Permission: "deny", Blocked: true, Message: "exit says no"}
	allow := Decision{Permission: "allow"}
	for _, tc := range []struct {
		name        string
		ds          []Decision
		wantRefused bool
		wantMessage string
	}{
		{"nothing decided", []Decision{{}, {}}, false, ""},
		{"allow", []Decision{allow}, false, ""},
		{"ask decides nothing", []Decision{{Permission: "ask"}, allow}, false, ""},
		{"deny before allow", []Decision{deny, allow}, true, "json says no"},
		{"deny after ask", []Decision{{Permission: "ask"}, deny}, true, "json says no"},
		{"deny before ask", []Decision{deny, {Permission: "ask"}}, true, "json says no"},
		{"deny after allow", []Decision{allow, deny}, true, "json says no"},
		{"a block outranks a deny", []Decision{deny, block}, true, "exit says no"},
		{"messages of denying hooks are concatenated", []Decision{deny, {Permission: "deny", Message: "and more"}}, true, "json says no\nand more"},
	} {
		refused, msg := Refusal(tc.ds)
		if refused != tc.wantRefused || msg != tc.wantMessage {
			t.Errorf("%s: Refusal = %v %q, want %v %q", tc.name, refused, msg, tc.wantRefused, tc.wantMessage)
		}
	}
}

func TestLoadReadsEntriesPerEventInOrder(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".cursor"), 0o755); err != nil {
		t.Fatal(err)
	}
	conf := `{"version":1,"hooks":{"preToolUse":[{"command":"a.sh"},{"command":"b.sh","failClosed":true,"matcher":"Shell","timeout":1.5}],"stop":[{"command":"c.sh"}]}}`
	if err := os.WriteFile(filepath.Join(dir, ".cursor", "hooks.json"), []byte(conf), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := c.Entries(PreToolUse); len(got) != 2 || got[0] != (Entry{Command: "a.sh"}) || got[1] != (Entry{Command: "b.sh", FailClosed: true, Matcher: "Shell", Timeout: 1500 * time.Millisecond}) {
		t.Errorf("preToolUse entries = %v, want [a.sh, b.sh failClosed]", got)
	}
	if got := c.Entries(SessionStart); len(got) != 0 {
		t.Errorf("an event with no hooks has entries: %v", got)
	}
	if empty, err := Load(t.TempDir()); err != nil || len(empty.Entries(PreToolUse)) != 0 {
		t.Errorf("no hooks.json is no hooks: %v %v", empty, err)
	}
}

// sr:proves hook-timeout/cursor
func TestAHookThatTimedOutIsIgnoredUnlessItFailsClosed(t *testing.T) {
	timedOut := corehooks.Outcome{Started: true, Exit: -1, TimedOut: true, Stdout: `{"permission":"deny"}`}
	if got := Interpret(BeforeShellExecution, Entry{Command: "h.sh", Timeout: time.Second}, timedOut); got != (Decision{}) {
		t.Errorf("a timed-out hook decides nothing, whatever it printed: %+v", got)
	}
	got := Interpret(BeforeShellExecution, Entry{Command: "h.sh", Timeout: time.Second, FailClosed: true}, timedOut)
	want := Decision{Permission: "deny", Blocked: true, Message: `Tool blocked because this hook is configured to fail closed (block when it fails). Hook "h.sh" execution failed: Hook script timed out after 1000ms`}
	if got != want {
		t.Errorf("fail closed: %+v, want %+v", got, want)
	}
}

func TestRefusalWording(t *testing.T) {
	failure, result := PreToolRefusal("msg")
	if failure != "msg" || result != "msg\n\nAgent note: Do not suggest workarounds to the blocked tool." {
		t.Errorf("PreToolRefusal = %q %q", failure, result)
	}
	failure, result = ShellRefusal("msg")
	want := "Command execution was blocked by a hook: msg\n\nTo view or modify configured hooks, go to Cursor Settings > Hooks.\n\nAgent note: Do not suggest workarounds to the blocked tool."
	if failure != want || result != want {
		t.Errorf("ShellRefusal = %q %q", failure, result)
	}
}
