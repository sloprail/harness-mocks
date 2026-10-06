package replay

import (
	"errors"
	"os"
	"path/filepath"
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

func assistantText(text string) map[string]any {
	return map[string]any{"type": "assistant", "message": map[string]any{"content": []any{map[string]any{"type": "text", "text": text}}}}
}

// What the model said before a Stop hook sent it on is a reply of its own (a pseudo-call of the
// adapter's), and its last answer is the final one.
func TestStopFeedbackIsAReply(t *testing.T) {
	got, err := modelTurns([]map[string]any{
		assistantText("DONE"), userRec("Stop hook feedback:\nWHY"), assistantText("DONE2"), userRec("Stop hook feedback:\nAGAIN"), assistantText("LAST"),
	})
	if err != nil || len(got.agent.Calls) != 2 || got.agent.Final != "LAST" {
		t.Fatalf("%+v %v", got, err)
	}
	if c := got.agent.Calls[1]; c.Tool != toolReply || c.Input["text"] != "DONE2" {
		t.Fatalf("%+v", c)
	}
	if _, err = modelTurns([]map[string]any{userRec("Stop hook feedback:\nWHY")}); err == nil {
		t.Fatal("feedback to a model that said nothing")
	}
	// the script makes the replies after the calls it has made, counting the feedback it was given
	s := Denormalize(core.Recording{Agent: core.Agent{Calls: got.agent.Calls, Final: got.agent.Final}}, "/d")
	if !strings.Contains(s.Script, "Stop hook feedback:") || !strings.Contains(s.Script, `"text":"DONE"`) || !strings.Contains(s.Script, `"text":"DONE2"`) {
		t.Fatal(s.Script)
	}
}

// A run that starts in a directory of the repository (through a symlink, say) has its project files
// there, and the symlink is made first.
func TestPrepareDir(t *testing.T) {
	repo := t.TempDir()
	dir, err := prepareDir(repo, ScenarioStep{Cwd: "link", Symlink: "link real", Settings: "{}", Hook: "#!/bin/sh\n"})
	if err != nil || dir != filepath.Join(repo, "link") {
		t.Fatalf("%v %v", dir, err)
	}
	if target, err := os.Readlink(filepath.Join(repo, "link")); err != nil || target != filepath.Join(repo, "real") {
		t.Fatalf("%v %v", target, err)
	}
	for _, f := range []string{"real/.claude/settings.json", "real/hook.sh"} {
		if _, err := os.Stat(filepath.Join(repo, f)); err != nil {
			t.Fatal(err)
		}
	}
	if dir, err = prepareDir(repo, ScenarioStep{Cwd: "sub"}); err != nil || dir != filepath.Join(repo, "sub") {
		t.Fatalf("%v %v", dir, err)
	}
	if _, err = prepareDir(repo, ScenarioStep{Symlink: "one"}); err == nil {
		t.Fatal("a symlink needs a name and a target")
	}
}

func TestStepSpecsReadsDirectoriesAndLinks(t *testing.T) {
	setup := t.TempDir()
	write := func(name, text string) {
		p := filepath.Join(setup, name)
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("prompt.txt", "one")
	write("cwd", "link")
	write("symlink", "link real\n")
	write("then/01/prompt.txt", "two")
	write("then/01/cwd", "sub")
	write("then/01/hook.sh", "#!/bin/sh\n")
	write("then-02-prompt.txt", "three")
	got, err := stepSpecs(setup)
	if err != nil || len(got) != 3 || got[0].cwd != "link" || got[0].symlink != "link real" || got[1].cwd != "sub" || got[1].hook == "" || got[2].prompt != "three" {
		t.Fatalf("%+v %v", got, err)
	}
	write("then/01/other", "x")
	var u *Unbuildable
	if _, err = stepSpecs(setup); !errors.As(err, &u) {
		t.Fatalf("a file the adapter does not install: %v", err)
	}
}

// A call the harness filled a default into is replayed as the model sent it: the stream names it.
func TestWireInputsReplaceTheTranscriptsInputs(t *testing.T) {
	stream := []map[string]any{{"type": "assistant", "wire_tool_inputs": map[string]any{"c1": map[string]any{"file_path": "/f"}}}}
	turns := turns{agent: core.Agent{Calls: []core.Call{{Tool: toolEdit, Input: map[string]any{"file_path": "/f", "replace_all": false}}, {Tool: core.ToolShell, Input: map[string]any{"command": "x"}}}}, ids: []string{"c1", "c2"}}
	got := withWireInputs(turns, wireInputs(stream))
	if _, has := got.agent.Calls[0].Input["replace_all"]; has || got.agent.Calls[1].Input["command"] != "x" {
		t.Fatalf("%+v", got.agent.Calls)
	}
	if _, has := turns.agent.Calls[0].Input["replace_all"]; !has {
		t.Fatal("the recording's own calls are left as they were")
	}
}

// A sample whose model read a setup file that now reads otherwise is replayed with the file as it read it.
func TestChangedSetupFiles(t *testing.T) {
	if got := unnumber("1\t#!/bin/sh\n2\techo x\n3\t"); got != "#!/bin/sh\necho x\n" {
		t.Fatalf("%q", got)
	}
	run := filepath.Join("..", "..", "snapshots", "runs", "hookmix")
	samples := sampleDirs(run)
	setup := filepath.Join(run, "setup")
	if got := changedSetupFiles(setup, samples[0]); len(got) != 0 {
		t.Fatalf("the sample that read nothing: %v", got)
	}
	if got := changedSetupFiles(setup, samples[1]); !strings.HasPrefix(got["hook.sh"], "#!/bin/sh\nIN=$(cat); D=") {
		t.Fatalf("%v", got)
	}
}

// The model's words name the recording's agent id; in the mock's output it stands for the mock's own, by order.
func TestRecordedAgentIDsStandForTheMocks(t *testing.T) {
	rec := []map[string]any{{"agent_id": "a1"}, {"agent_id": "a2"}}
	mock := []map[string]any{{"agent_id": "m1", "text": "path/agent-a1 and agent-a2"}, {"agent_id": "m2"}}
	got := withRecordedAgentIDs(mock, agentIDs(rec), agentIDs(mock))
	if got[0]["text"] != "path/agent-m1 and agent-m2" || mock[0]["text"] != "path/agent-a1 and agent-a2" {
		t.Fatalf("%v", got)
	}
	if same := withRecordedAgentIDs(mock, []string{"a1"}, agentIDs(mock)); same[0]["text"] != mock[0]["text"] {
		t.Fatal("a different number of agents is left as it is")
	}
}
