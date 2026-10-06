package replay

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	core "github.com/sloprail/harness-mocks/internal/replay"
)

func userRec(text string) map[string]any {
	return map[string]any{"type": "user", "message": map[string]any{"content": text}}
}

func TestParseStepArgs(t *testing.T) {
	s, err := parseStepArgs("--continue\n--max-turns\n2\n")
	if err != nil || !s.cont || !reflect.DeepEqual(s.args, []string{"--max-turns", "2"}) {
		t.Fatalf("%+v %v", s, err)
	}
	if s, err = parseStepArgs("--resume\nabc\n"); err != nil || s.resume != "abc" {
		t.Fatalf("%+v %v", s, err)
	}
	if s, err = parseStepArgs("--resume\nabc\n--fork-session\n--session-id\nxyz\n"); err != nil || !s.fork || s.newID != "xyz" {
		t.Fatalf("%+v %v", s, err)
	}
	if got := mainMockArgs(s); !reflect.DeepEqual(got, []string{"--resume", "abc", "--session-id", "xyz", "--fork-session"}) {
		t.Fatalf("%v", got)
	}
	if got := stepMockArgs(s); !reflect.DeepEqual(got, []string{"--resume", "<SESSION>", "--fork-session", "--session-id", "xyz"}) {
		t.Fatalf("%v", got)
	}
	var u *Unbuildable
	if _, err = parseStepArgs("--system-prompt\nx\n"); !errors.As(err, &u) {
		t.Fatalf("an unmodelled flag: %v", err)
	}
}

// A preparation's claude runs are the earlier runs: a plugin command is not one, and one the
// adapter cannot read is not replayed.
func TestEarlierSpecs(t *testing.T) {
	got, err := earlierSpecs("#!/bin/sh\n# a comment about claude\nclaude plugin install x\nHOOK_LOG=/dev/null claude -p --model haiku --session-id s1 --name n 'it'\\''s ONE' </dev/null >/dev/null\n")
	if err != nil || len(got) != 1 || got[0].prompt != "it's ONE" || got[0].newID != "s1" {
		t.Fatalf("%+v %v", got, err)
	}
	var u *Unbuildable
	if _, err = earlierSpecs("claude -p --model haiku ONE\n"); !errors.As(err, &u) {
		t.Fatalf("a run with no session id or quoted prompt: %v", err)
	}
	if got, err = earlierSpecs("mkdir x\n"); err != nil || len(got) != 0 {
		t.Fatalf("%+v %v", got, err)
	}
}

// A run that resumes the first session works in it; one that names a session the recording does
// not hold, or continues one other than the first, is not replayed.
func TestStepThreads(t *testing.T) {
	got, err := stepThreads([]stepSpec{{}, {cont: true}, {resume: "s1"}, {newID: "s2"}, {resume: "s1", fork: true, newID: "s3"}}, "s1")
	if err != nil || !reflect.DeepEqual(got, []string{"s1", "s1", "s1", "s2", "s3"}) {
		t.Fatalf("%v %v", got, err)
	}
	for _, bad := range []stepSpec{{resume: "other"}, {}} {
		if _, err := stepThreads([]stepSpec{{}, bad}, "s1"); err == nil {
			t.Fatalf("%+v should not be replayed", bad)
		}
	}
	if _, err := stepThreads([]stepSpec{{}, {newID: "s2"}, {cont: true}}, "s1"); err == nil {
		t.Fatal("continuing a session other than the first")
	}
}

// Runs in one session's transcript are what follows each one's own prompt, up to the next one's.
func TestStepRecords(t *testing.T) {
	recs := []map[string]any{userRec("ONE"), {"type": "assistant", "n": 1}, userRec("TWO"), {"type": "assistant", "n": 2}}
	got, err := stepRecords([]stepSpec{{prompt: "ONE"}, {prompt: "TWO"}}, []string{"s", "s"}, map[string][]map[string]any{"s": recs})
	if err != nil || len(got) != 2 || got[0][0]["n"] != 1 || got[1][0]["n"] != 2 || len(got[0]) != 1 || len(got[1]) != 1 {
		t.Fatalf("%v %v", got, err)
	}
	if _, err = stepRecords([]stepSpec{{prompt: "ONE"}, {prompt: "THREE"}}, []string{"s", "s"}, map[string][]map[string]any{"s": recs}); err == nil {
		t.Fatal("a prompt that is not in the transcript")
	}
}

// A run that resumes a session finds the earlier runs' tool results in its file, so its script
// skips as many calls as they made.
func TestResumedScriptSkipsEarlierCalls(t *testing.T) {
	call := core.Call{Tool: core.ToolShell, Input: map[string]any{"command": "true"}}
	s := Denormalize(core.Recording{
		Agent: core.Agent{Calls: []core.Call{call, call}},
		Then:  []core.Step{{Prompt: "p", Args: []string{"--continue"}, Agent: core.Agent{Calls: []core.Call{call}}}, {Prompt: "q", Args: []string{"--session-id", "x"}}},
	}, "/d")
	if !strings.Contains(s.Then[0].Script, "$((n+1-2))p") || !strings.Contains(s.Then[1].Script, "$((n+1-0))p") {
		t.Fatalf("%s\n%s", s.Then[0].Script, s.Then[1].Script)
	}
}
