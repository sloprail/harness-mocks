package hooks

import (
	"testing"

	corehooks "github.com/sloprail/harness-mocks/internal/hooks"
)

func out(exit int, stdout, stderr string) corehooks.Outcome {
	return corehooks.Outcome{Command: "hook", Exit: exit, Started: true, Stdout: stdout, Stderr: stderr}
}

const denyJSON = `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"JSON reason"}}`

// Codex 0.159.3 recorded (runs/hook-exit-json, runs/hook-exit-codes): exit 0
// is accepted; exit 2 blocks with the stderr as the reason, and JSON printed
// beside it is not read (the stderr won over a deny's reason); any other exit
// status does not block, whatever it printed, a deny included; a hook that
// cannot start or times out does not block either.
// sr:proves hook-exit-code-semantics/codex
func TestInterpretExitStatuses(t *testing.T) {
	if d := Interpret(PreToolUse, out(0, "", "quiet stderr")); d != (Decision{}) {
		t.Errorf("exit 0: %+v", d)
	}
	d := Interpret(PreToolUse, out(2, denyJSON, "the stderr wins\n"))
	if !d.Blocked || d.BlockReason != "the stderr wins" || d.Denied || d.Error != "" {
		t.Errorf("exit 2 with JSON: %+v", d)
	}
	for _, code := range []int{1, 3, 127} {
		d := Interpret(PreToolUse, out(code, denyJSON, "stderr"))
		if d.Blocked || d.Denied || d.Error == "" || d.Permission != "" {
			t.Errorf("exit %d with a deny printed: %+v, want a non-blocking error", code, d)
		}
	}
	if d := Interpret(Stop, corehooks.Outcome{Command: "missing"}); d.Blocked || d.Error == "" {
		t.Errorf("not started: %+v", d)
	}
	if d := Interpret(Stop, corehooks.Outcome{Command: "slow", Started: true, TimedOut: true, Exit: -1}); d.Blocked || d.Error == "" {
		t.Errorf("timed out: %+v", d)
	}
}

// What exit 0 prints (runs/hook-exit-json, runs/stops): a deny, or the older
// block shape, refuses; malformed or schema-invalid JSON, JSON over several
// lines, and the decisions Codex parses but does not support (ask) fail the
// hook without deciding; plain text is ignored by PreToolUse.
// sr:proves hook-exit-code-semantics/codex
func TestInterpretWhatExitZeroPrints(t *testing.T) {
	d := Interpret(PreToolUse, out(0, denyJSON, ""))
	if !d.Denied || d.DenyReason != "JSON reason" || d.Permission != "deny" || d.Blocked {
		t.Errorf("deny: %+v", d)
	}
	d = Interpret(PreToolUse, out(0, `{"decision":"block","reason":"old shape"}`, ""))
	if !d.Denied || d.DenyReason != "old shape" {
		t.Errorf("block: %+v", d)
	}
	for name, stdout := range map[string]string{
		"malformed":      `{not json at all}`,
		"unclosed":       `{"unclosed": 1`,
		"schema-invalid": `{"decision": 42}`,
		"two objects":    "{\"x\": 1}\n{\"decision\": \"block\", \"reason\": \"lines\"}",
		"ask":            `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"ask"}}`,
	} {
		if d := Interpret(PreToolUse, out(0, stdout, "")); d.Denied || d.Blocked || d.Error == "" {
			t.Errorf("%s: %+v, want a failed hook that decides nothing", name, d)
		}
	}
	for name, stdout := range map[string]string{"array": `[1, 2]`, "string": `"just a string"`, "text": "note"} {
		if d := Interpret(PreToolUse, out(0, stdout, "")); d != (Decision{}) {
			t.Errorf("%s: %+v, want it ignored", name, d)
		}
	}
}

// Plain text on exit 0 is context for SessionStart and UserPromptSubmit
// (runs/hook-exit-codes: the model answered with the hook's secret word), and
// invalid for Stop.
// sr:proves hook-exit-code-semantics/codex
func TestInterpretPlainTextByEvent(t *testing.T) {
	for _, ev := range []Event{SessionStart, UserPromptSubmit} {
		if d := Interpret(ev, out(0, "The secret word is BANANA.\n", "")); d.Context != "The secret word is BANANA." {
			t.Errorf("%s: %+v", ev, d)
		}
	}
	if d := Interpret(Stop, out(0, "text", "")); d.Error == "" || d.Context != "" {
		t.Errorf("Stop: %+v", d)
	}
	ctxJSON := `{"hookSpecificOutput":{"hookEventName":"SessionStart","additionalContext":"SS-CTX"}}`
	if d := Interpret(SessionStart, out(0, ctxJSON, "")); d.Context != "SS-CTX" {
		t.Errorf("additionalContext: %+v", d)
	}
	if d := Interpret(Stop, out(0, `{"decision":"block","reason":"again"}`, "")); !d.Denied || d.DenyReason != "again" {
		t.Errorf("Stop block: %+v", d)
	}
}

// Several hooks decide one call (runs/pretool-decisions: one hook allowed, one
// denied, one asked): a deny stands over an allow, and an ask refuses nothing.
// sr:proves pretooluse-refusal/codex
func TestRefusalDenyStandsOverAllowAndAskRefusesNothing(t *testing.T) {
	allow := Interpret(PreToolUse, out(0, `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"allow"}}`, ""))
	deny := Interpret(PreToolUse, out(0, denyJSON, ""))
	ask := Interpret(PreToolUse, out(0, `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"ask"}}`, ""))
	blocked := Interpret(PreToolUse, out(2, "", "exit-2 reason\n"))
	if r, _ := Refusal([]Decision{allow, ask}); r {
		t.Error("allow and ask refused a call")
	}
	if r, why := Refusal([]Decision{allow, deny, ask}); !r || why != "JSON reason" {
		t.Errorf("deny among others: (%v, %q)", r, why)
	}
	if r, why := Refusal([]Decision{blocked}); !r || why != "exit-2 reason" {
		t.Errorf("exit 2: (%v, %q), want the stderr as the reason", r, why)
	}
}

func TestMatcherAndEvents(t *testing.T) {
	cfg := Config{PreToolUse: {{Matcher: "^Bash$", Handlers: []Handler{{Command: "true"}}}}}
	iv := &Invoker{Config: cfg, Dir: t.TempDir(), Environ: []string{"PATH=/usr/bin:/bin"}}
	if got := iv.Fire(t.Context(), PreToolUse, "Bash", nil); len(got) != 1 || got[0].Exit != 0 {
		t.Errorf("Bash: %+v", got)
	}
	if got := iv.Fire(t.Context(), PreToolUse, "apply_patch", nil); len(got) != 0 {
		t.Errorf("apply_patch matched ^Bash$: %+v", got)
	}
	iv.Config = Config{Stop: {{Matcher: "never", Handlers: []Handler{{Command: "true"}}}}}
	if got := iv.Fire(t.Context(), Stop, "", nil); len(got) != 1 {
		t.Errorf("Stop ignores its matcher: %+v", got)
	}
}
