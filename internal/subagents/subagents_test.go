package subagents

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/sloprail/harness-mocks/internal/tasks"
)

func layout() Layout {
	return Layout{SessionExt: ".jsonl", Dir: "subagents", Prefix: "agent-", Ext: ".jsonl"}
}

func TestLayoutPath_BesideTheSessionNeverIn(t *testing.T) {
	got := layout().Path("/p/proj/sess.jsonl", "a1")
	if got != "/p/proj/sess/subagents/agent-a1.jsonl" {
		t.Fatalf("Path = %s", got)
	}
	if got == "/p/proj/sess.jsonl" {
		t.Fatal("the sub-agent's records would go into the session's file")
	}
}

func TestPlace_OneDeeperNamingTheParent(t *testing.T) {
	if p := Place(Parent{}); p != (Placement{Depth: 1}) {
		t.Fatalf("main thread's child = %+v", p)
	}
	if p := Place(Parent{ID: "a1", Depth: 1}); p != (Placement{Depth: 2, ParentID: "a1"}) {
		t.Fatalf("nested = %+v", p)
	}
}

func TestExecute_StartCannotBlockAndStopReruns(t *testing.T) {
	var log []string
	runs := 0
	h := Hooks{
		Start: func() { log = append(log, "start") },
		Stop: func(active bool, last string) (bool, string) {
			log = append(log, "stop")
			if !active {
				return true, "do it again"
			}
			return false, ""
		},
		OnRerun: func(reason string, turn int) { log = append(log, "rerun:"+reason) },
	}
	out := Execute(h, 8, func() Outcome { runs++; return Outcome{LastAssistant: "text", ToolUses: 2} })
	if runs != 2 || out.ToolUses != 4 {
		t.Fatalf("runs=%d toolUses=%d, want a re-run and the tool uses summed", runs, out.ToolUses)
	}
	want := []string{"start", "stop", "rerun:do it again", "stop"}
	if len(log) != len(want) {
		t.Fatalf("log = %v", log)
	}
	for i := range want {
		if log[i] != want[i] {
			t.Fatalf("log = %v, want %v", log, want)
		}
	}
}

func TestBegin_StartsNowAndFinishesLater(t *testing.T) {
	var events []string
	finish := Begin(Hooks{
		Start: func() { events = append(events, "start") },
		Stop:  func(bool, string) (bool, string) { events = append(events, "stop"); return false, "" },
	})
	if len(events) != 1 || events[0] != "start" {
		t.Fatalf("Begin should fire only the start hook: %v", events)
	}
	out := finish(0, func() Outcome { events = append(events, "run"); return Outcome{FinalText: "x"} })
	if out.FinalText != "x" || len(events) != 3 || events[1] != "run" || events[2] != "stop" {
		t.Fatalf("finishing runs then stops, without a second start: %v %+v", events, out)
	}
}

func TestExecute_StopsAtTheBlockCap(t *testing.T) {
	stops, capped := 0, 0
	h := Hooks{
		Stop:  func(bool, string) (bool, string) { stops++; return true, "no" },
		OnCap: func(n int) { capped = n },
	}
	runs := 0
	Execute(h, 3, func() Outcome { runs++; return Outcome{} })
	if stops != 4 || runs != 4 || capped != 3 {
		t.Fatalf("stops=%d runs=%d capped=%d: the block after the 3rd re-run must be overridden", stops, runs, capped)
	}
	runs = 0
	h.Stop = func(bool, string) (bool, string) { return runs < 12, "x" }
	Execute(h, 0, func() Outcome { runs++; return Outcome{} })
	if runs != 12 {
		t.Fatalf("with no cap it ran %d times", runs)
	}
}

func TestStop_ListsTheSessionsTasksNotOnlyTheSubagents(t *testing.T) {
	r := tasks.NewRegistry()
	defer r.Shutdown()
	ag := tasks.NewTask(tasks.Agent, "ag")
	ag.Owner = "someone-else"
	r.StartAgent(ag, func(ctx context.Context) { <-ctx.Done() })
	f := Stop("/t/agent.jsonl", "bye", r)
	if f.TranscriptPath != "/t/agent.jsonl" || f.LastMessage != "bye" || len(f.Tasks) != 1 || f.Tasks[0] != ag {
		t.Fatalf("facts = %+v", f)
	}
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.email=a@b.c", "-c", "user.name=n", "-c", "commit.gpgsign=false"}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func wl() WorktreeLayout {
	return WorktreeLayout{Dir: ".claude/worktrees", Prefix: "agent-", BranchPrefix: "worktree-agent-"}
}

func TestIsolate_RealWorktreeOnANewBranch(t *testing.T) {
	repo := t.TempDir()
	git(t, repo, "init", "-q")
	git(t, repo, "commit", "-q", "--allow-empty", "-m", "init")
	iso := Isolate(repo, "a1", wl(), BindGit(context.Background(), repo))
	want := filepath.Join(repo, ".claude/worktrees/agent-a1")
	if iso.Cwd != want || iso.Worktree == nil || iso.Worktree.Branch != "worktree-agent-a1" || iso.Worktree.Path != want {
		t.Fatalf("isolation = %+v", iso)
	}
	if _, err := os.Stat(filepath.Join(want, ".git")); err != nil {
		t.Fatalf("not a git worktree: %v", err)
	}
	out, _ := exec.Command("git", "-C", repo, "branch", "--list", "worktree-agent-a1").Output()
	if len(out) == 0 {
		t.Fatal("the new branch was not created")
	}
}

func TestIsolate_FallsBackToAPlainDirectoryThenToTheParent(t *testing.T) {
	plain := t.TempDir()
	iso := Isolate(plain, "a2", wl(), BindGit(context.Background(), plain))
	if iso.Worktree != nil || iso.Cwd != filepath.Join(plain, ".claude/worktrees/agent-a2") || len(iso.Notes) != 1 {
		t.Fatalf("not a repo: %+v", iso)
	}
	if st, err := os.Stat(iso.Cwd); err != nil || !st.IsDir() {
		t.Fatalf("plain directory missing: %v", err)
	}
	blocker := t.TempDir()
	if err := os.WriteFile(filepath.Join(blocker, ".claude"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	iso = Isolate(blocker, "a3", wl(), func(string, string) error { return errors.New("no") })
	if iso.Cwd != blocker || iso.Worktree != nil {
		t.Fatalf("unusable directory: %+v", iso)
	}
}

func TestHandBack_IndentsTheReportUnderTheFrame(t *testing.T) {
	if got := HandBack("FRAME", "NONE", "one\r\ntwo\u2028three"); got != "FRAME\n  one\n  two\n  three" {
		t.Fatalf("report = %q", got)
	}
	if got := HandBack("FRAME", "NONE", ""); got != "FRAME\n  NONE" {
		t.Fatalf("empty = %q", got)
	}
}

func TestWorktreeHook(t *testing.T) {
	boom := errors.New("hook failed")
	if _, ran, err := WorktreeHook(false, func() (string, bool, error) { return "", true, boom }); !ran || err != boom {
		t.Fatalf("ran=%v err=%v: a failing hook is its error", ran, err)
	}
	if _, ran, err := WorktreeHook(true, func() (string, bool, error) { return "", false, nil }); ran || err != nil {
		t.Fatalf("ran=%v err=%v: no hook configured is not a failure", ran, err)
	}
	if path, ran, err := WorktreeHook(true, func() (string, bool, error) { return "noise\n/w/made\n", true, nil }); !ran || err != nil || path != "/w/made" {
		t.Fatalf("path=%q ran=%v err=%v", path, ran, err)
	}
	if _, ran, err := WorktreeHook(true, func() (string, bool, error) { return "\n", true, nil }); !ran || err == nil {
		t.Fatalf("ran=%v err=%v: a create hook that prints no path fails the creation", ran, err)
	}
	if path, _, err := WorktreeHook(false, func() (string, bool, error) { return "ignored", true, nil }); err != nil || path != "" {
		t.Fatalf("path=%q err=%v: a remove hook returns no path", path, err)
	}
}

func TestCanDispatch_StopsAtTheLimit(t *testing.T) {
	for _, c := range []struct {
		p     Parent
		limit int
		want  bool
	}{
		{Parent{}, 0, true},
		{Parent{ID: "b", Depth: 2}, 0, true},
		{Parent{ID: "c", Depth: 3}, 0, false},
		{Parent{ID: "a", Depth: 1}, 2, true},
		{Parent{ID: "b", Depth: 2}, 2, false},
		{Parent{ID: "a", Depth: 1}, 1, false},
	} {
		if got := c.p.CanDispatch(c.limit); got != c.want {
			t.Errorf("%+v limit %d: %v, want %v", c.p, c.limit, got, c.want)
		}
	}
}
