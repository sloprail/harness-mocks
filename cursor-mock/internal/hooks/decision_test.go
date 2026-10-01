package hooks

import (
	"os"
	"path/filepath"
	"testing"

	corehooks "github.com/sloprail/harness-mocks/internal/hooks"
)

func run(verdict corehooks.Verdict, stdout, stderr string) corehooks.Run {
	return corehooks.Run{Verdict: verdict, Stdout: stdout, Stderr: stderr, ExitCode: map[corehooks.Verdict]int{corehooks.Blocked: 2, corehooks.NonBlockingError: 1}[verdict]}
}

// sr:proves hook-exit-code-semantics/cursor
func TestAFailClosedHookBlocksOnAnyFailure(t *testing.T) {
	h := Entry{Command: "h.sh", FailClosed: true}
	strict := corehooks.Run{Verdict: corehooks.Blocked, ExitCode: 3, Stderr: "boom\n"}
	got := Interpret(BeforeShellExecution, h, strict)
	want := Decision{Permission: "deny", Blocked: true, Message: `Tool blocked because this hook is configured to fail closed (block when it fails). Hook "h.sh" failed with exit code 3: boom`}
	if got != want {
		t.Errorf("non-zero status: %+v, want %+v", got, want)
	}
	got = Interpret(PreToolUse, h, corehooks.Run{Verdict: corehooks.Accepted})
	want = Decision{Permission: "deny", Message: `Tool blocked because this hook is configured to fail closed (block when it fails). Hook "h.sh" returned no output.`}
	if got != want {
		t.Errorf("no output: %+v, want %+v", got, want)
	}
	if got := Interpret(AfterShellExecution, h, corehooks.Run{Verdict: corehooks.Accepted}); got != (Decision{}) {
		t.Errorf("silence from an event with nothing to refuse: %+v", got)
	}
}

// sr:proves hook-exit-code-semantics/cursor
func TestInterpretByExitStatusAndOutput(t *testing.T) {
	for _, tc := range []struct {
		name  string
		event Event
		run   corehooks.Run
		want  Decision
	}{
		{"exit 2 blocks with stderr", BeforeShellExecution, run(corehooks.Blocked, "", "no\n"), Decision{Permission: "deny", Blocked: true, Message: "Hook blocked with message: no"}},
		{"other status fails open, output unread", BeforeShellExecution, run(corehooks.NonBlockingError, `{"permission":"deny"}`, ""), Decision{}},
		{"silence allows", BeforeShellExecution, run(corehooks.Accepted, "  \n", ""), Decision{}},
		{"json deny", PreToolUse, run(corehooks.Accepted, `{"permission":"deny","user_message":"why"}`, ""), Decision{Permission: "deny", Message: "why"}},
		{"json allow", PreToolUse, run(corehooks.Accepted, `{"permission":"allow"}`, ""), Decision{Permission: "allow"}},
		{"text blocks", BeforeShellExecution, run(corehooks.Accepted, "not json", ""), Decision{Permission: "deny", Message: `Hook "h.sh" returned invalid JSON. The command was blocked for safety.`}},
		{"unknown permission blocks", PreToolUse, run(corehooks.Accepted, `{"permission":"maybe"}`, ""), Decision{Permission: "deny", Message: `Hook "h.sh" returned invalid JSON. The command was blocked for safety.`}},
		{"a hook on another event is not read", PostToolUse, run(corehooks.Accepted, "not json", ""), Decision{}},
	} {
		if got := Interpret(tc.event, Entry{Command: "h.sh"}, tc.run); got != tc.want {
			t.Errorf("%s: Interpret = %+v, want %+v", tc.name, got, tc.want)
		}
	}
}

// sr:proves pretooluse-refusal/cursor
func TestDecideDenyWinsAndABlockOutranksIt(t *testing.T) {
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
		{"deny after allow", []Decision{allow, deny}, true, "json says no"},
		{"a block outranks a deny", []Decision{deny, block}, true, "exit says no"},
		{"messages of denying hooks are concatenated", []Decision{deny, {Permission: "deny", Message: "and more"}}, true, "json says no\nand more"},
	} {
		refused, msg := Decide(tc.ds)
		if refused != tc.wantRefused || msg != tc.wantMessage {
			t.Errorf("%s: Decide = %v %q, want %v %q", tc.name, refused, msg, tc.wantRefused, tc.wantMessage)
		}
	}
}

func TestLoadReadsCommandsPerEventInOrder(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".cursor"), 0o755); err != nil {
		t.Fatal(err)
	}
	conf := `{"version":1,"hooks":{"preToolUse":[{"command":"a.sh"},{"command":"b.sh","matcher":"Shell"}],"stop":[{"command":"c.sh"}]}}`
	if err := os.WriteFile(filepath.Join(dir, ".cursor", "hooks.json"), []byte(conf), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := c.Entries(PreToolUse); len(got) != 2 || got[0].Command != "a.sh" || got[1].Command != "b.sh" {
		t.Errorf("preToolUse commands = %v, want [a.sh b.sh]", got)
	}
	if got := c.Entries(SessionStart); len(got) != 0 {
		t.Errorf("an event with no hooks has commands: %v", got)
	}
	if empty, err := Load(t.TempDir()); err != nil || len(empty.Entries(PreToolUse)) != 0 {
		t.Errorf("no hooks.json is no hooks: %v %v", empty, err)
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
