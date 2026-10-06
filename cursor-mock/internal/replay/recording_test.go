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
	_, err := Adapter{}.Load(runDir("session-fork"))
	if u, ok := err.(*Unbuildable); !ok || !strings.Contains(u.Reason, "then-01") {
		t.Fatalf("err = %v, want an Unbuildable naming the later step", err)
	}
}

// The script plays each step where the session file stands: a response of text
// and two calls adds two records (the text is in the first call's).
func TestScriptStepsAdvanceByTheRecordsTheyAdd(t *testing.T) {
	said := "go"
	s := script("t", []step{
		{said: &said, calls: []scriptCall{{Name: "Shell", Input: map[string]any{"command": "a"}}, {Name: "Shell", Input: map[string]any{"command": "b"}}}},
		{said: &said},
	})
	if !strings.Contains(s, "\n0) printf") || !strings.Contains(s, "\n2) printf") {
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

// What a hook script logs under a tag of its own, not a hook event, is part of
// the event whose lines surround it, so the order the concurrent hooks logged in
// does not matter.
func TestConcurrentGroupsAScriptsOwnTagWithItsEvent(t *testing.T) {
	objs := []map[string]any{
		{"hook_event_name": "beforeReadFile"}, {"hook_result": map[string]any{"event": "closed"}}, {"hook_event_name": "beforeReadFile"},
		{"hook_event_name": "postToolUse"},
	}
	got := concurrent(objs, []string{"c", "a", "b", "z"})
	if strings.Join(got, "") != "abcz" {
		t.Fatalf("got %v", got)
	}
}

// A named sample of a run is the one replayed, so every sample can be.
func TestLoadReadsTheNamedSample(t *testing.T) {
	samples, _ := filepath.Glob(filepath.Join(runDir("additional-context"), "samples", "*"))
	if len(samples) == 0 {
		t.Fatal("no sample")
	}
	if _, err := (Adapter{Sample: filepath.Base(samples[0])}).Load(runDir("additional-context")); err != nil {
		t.Fatal(err)
	}
	if _, err := (Adapter{Sample: "19700101-000000"}).Load(runDir("additional-context")); err == nil {
		t.Fatal("a sample that does not exist must not load")
	}
}

// A flag of a setup's args the mock models is passed on with its value; any
// other is refused, and so is a value that names an earlier step's session.
func TestFlagWordsPassOnWhatTheMockModelsAndRefuseTheRest(t *testing.T) {
	got, err := flagWords("--add-dir\n../second-root\n--approve-mcps\n")
	if err != nil || strings.Join(got, " ") != "--add-dir ../second-root --approve-mcps" {
		t.Fatalf("got %v, %v", got, err)
	}
	for _, bad := range []string{"--sandbox\nx\n", "--add-dir\n", "--resume\n<SESSION>\n"} {
		if _, err := flagWords(bad); err == nil {
			t.Errorf("%q must be refused", bad)
		}
	}
}

// A recorded MCP call is a call of the server's tool with the model's arguments,
// the lookup before it left to the mock; Grep and Delete are the unified search
// and delete.
func TestLoadMapsMCPGrepAndDelete(t *testing.T) {
	rec, err := Adapter{}.Load(runDir("hook-matchers-mcp"))
	if err != nil {
		t.Fatal(err)
	}
	c := rec.Agent.Calls[0]
	if c.Tool != core.ToolMCP || c.Input["server"] != "local" || c.Input["tool"] != "echo" || len(rec.Agent.Calls) != 1 {
		t.Fatalf("calls %+v", rec.Agent.Calls)
	}
	if s := Denormalize(rec, "<scripts>", nil); !strings.Contains(s.Script, "mcp__local__echo") {
		t.Fatalf("script:\n%s", s.Script)
	}
	rec, err = Adapter{}.Load(runDir("hook-matchers-grep-delete"))
	if err != nil || rec.Agent.Calls[0].Tool != core.ToolSearchFiles || rec.Agent.Calls[1].Tool != core.ToolDeleteFile {
		t.Fatalf("calls %+v, %v", rec.Agent.Calls, err)
	}
}

// The thoughts a recording holds are put on the responses they were had in, by
// the response each names; a response that had none gets none (runs/symlinked-cwd:
// the first response thought and the second did not).
func TestLoadPutsTheThoughtsOnTheirResponses(t *testing.T) {
	rec, err := Adapter{}.Load(runDir("agent-input-validation"))
	if err != nil {
		t.Fatal(err)
	}
	c := rec.Agent.Calls[0]
	if c.Thinking == nil || c.Thinking.Fields["model_id"] == nil || rec.Agent.FinalThinking == nil {
		t.Fatalf("call %+v, final %+v", c, rec.Agent.FinalThinking)
	}
	if s := Denormalize(rec, "<scripts>", nil); !strings.Contains(s.Script, `"type":"thinking"`) {
		t.Fatalf("script:\n%s", s.Script)
	}
	rec, err = Adapter{}.Load(runDir("symlinked-cwd"))
	if err != nil {
		t.Fatal(err)
	}
	if c := rec.Agent.Calls[0]; c.Thinking == nil || rec.Agent.FinalThinking != nil {
		t.Fatalf("call %+v, final %+v: the first response thought, the answer did not", c, rec.Agent.FinalThinking)
	}
}

// A response's thought and its preToolUse are told together, in either order; the
// events around them keep theirs.
func TestConcurrentGroupsAThoughtWithThePreToolUseBesideIt(t *testing.T) {
	ev := func(name string) map[string]any { return map[string]any{"hook_event_name": name} }
	objs := []map[string]any{ev("sessionStart"), ev("preToolUse"), ev("afterAgentThought"), ev("beforeShellExecution")}
	if got := concurrent(objs, []string{"s", "pre", "T", "b"}); strings.Join(got, "") != "sTpreb" {
		t.Fatalf("pre then thought: %v", got)
	}
	objs = []map[string]any{ev("sessionStart"), ev("afterAgentThought"), ev("preToolUse"), ev("beforeShellExecution")}
	if got := concurrent(objs, []string{"s", "T", "pre", "b"}); strings.Join(got, "") != "sTpreb" {
		t.Fatalf("thought then pre: %v", got)
	}
	objs = []map[string]any{ev("afterAgentThought"), ev("beforeShellExecution"), ev("preToolUse")}
	if got := concurrent(objs, []string{"T", "b", "pre"}); strings.Join(got, "") != "Tbpre" {
		t.Fatalf("a thought is not concurrent with what is not a preToolUse: %v", got)
	}
}
