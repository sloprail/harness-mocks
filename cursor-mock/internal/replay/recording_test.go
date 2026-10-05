package replay

import (
	"path/filepath"
	"strings"
	"testing"

	core "github.com/sloprail/harness-mocks/internal/replay"
)

func runDir(name string) string { return filepath.Join("..", "..", "snapshots", "runs", name) }

// A recording is read into the calls the model made: what it said before them,
// and the calls of one response together.
func TestLoadReadsTheModelsTurns(t *testing.T) {
	rec, err := Adapter{}.Load(runDir("additional-context"))
	if err != nil {
		t.Fatal(err)
	}
	if len(rec.Agent.Calls) != 2 || rec.Agent.Final == "" {
		t.Fatalf("calls %d, final %q", len(rec.Agent.Calls), rec.Agent.Final)
	}
	if c := rec.Agent.Calls[0]; c.Tool != core.ToolShell || c.Input["command"] != "echo FIRST" {
		t.Fatalf("first call %+v", c)
	}
}

// Two calls of one response are in the same turn, and a sub-agent is attached to
// the call that started it by its prompt, nested ones in turn.
func TestLoadGroupsTheCallsOfOneResponseAndAttachesSubagents(t *testing.T) {
	rec, err := Adapter{}.Load(runDir("nested-subagents"))
	if err != nil {
		t.Fatal(err)
	}
	spawn := rec.Agent.Calls[0]
	if spawn.Tool != core.ToolSpawn || spawn.Sub == nil || spawn.Sub.Calls[0].Sub == nil {
		t.Fatalf("spawn %+v", spawn)
	}
	if got := spawn.Sub.Calls[0].Sub.Final; got != "LEAF" {
		t.Fatalf("the nested sub-agent said %q", got)
	}
}

// What the mock has no tool for is refused, never guessed.
func TestLoadRefusesATool(t *testing.T) {
	_, err := Adapter{}.Load(runDir("file-tools"))
	if u, ok := err.(*Unbuildable); !ok || !strings.Contains(u.Reason, "StrReplace") {
		t.Fatalf("err = %v, want an Unbuildable naming the tool", err)
	}
}

// A setup the adapter does not install is refused, naming the file.
func TestLoadRefusesASetupItDoesNotInstall(t *testing.T) {
	_, err := Adapter{}.Load(runDir("plugin-hooks"))
	if u, ok := err.(*Unbuildable); !ok || !strings.Contains(u.Reason, "args") {
		t.Fatalf("err = %v, want an Unbuildable naming args", err)
	}
}

// The script plays each step where the session file stands: a response of text
// and two calls adds three records.
func TestScriptStepsAdvanceByTheRecordsTheyAdd(t *testing.T) {
	said := "go"
	s := script("t", []step{
		{said: &said, calls: []scriptCall{{Name: "Shell", Input: map[string]any{"command": "a"}}, {Name: "Shell", Input: map[string]any{"command": "b"}}}},
		{said: &said},
	})
	if !strings.Contains(s, "\n0) printf") || !strings.Contains(s, "\n3) printf") {
		t.Fatalf("script:\n%s", s)
	}
}

// The hooks of one event, and what their scripts logged for it, are put in a
// fixed order among themselves, and the events keep theirs.
func TestConcurrentSortsOnlyWithinAnEvent(t *testing.T) {
	objs := []map[string]any{
		{"hook_event_name": "preToolUse"}, {"hook_result": map[string]any{"event": "preToolUse"}},
		{"hook_event_name": "postToolUse"},
		{"hook_event_name": "preToolUse"},
	}
	got := concurrent(objs, []string{"b", "a", "z", "c"})
	if strings.Join(got, "") != "abzc" {
		t.Fatalf("got %v", got)
	}
}
