package tasks

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func startSh(t *testing.T, r *Registry, id, owner, script string) (*Task, string) {
	t.Helper()
	out, err := os.Create(filepath.Join(t.TempDir(), id+".out"))
	if err != nil {
		t.Fatal(err)
	}
	task := NewTask(Command, id)
	task.Owner = owner
	err = r.StartCommand(task, CommandSpec{
		Argv: []string{"/bin/sh", "-c", script}, Dir: t.TempDir(), Env: os.Environ(), Out: out,
		Trailer: func(code int, killed bool) string {
			if killed {
				return "\n[killed]\n"
			}
			return fmt.Sprintf("\n[exited with code %d]\n", code)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return task, out.Name()
}

func wait(t *testing.T, task *Task) {
	t.Helper()
	select {
	case <-task.Done():
	case <-time.After(10 * time.Second):
		t.Fatalf("task %s did not finish", task.ID)
	}
}

func TestStartCommand_RunsConcurrentlyAndRecordsHowItEnded(t *testing.T) {
	r := NewRegistry()
	defer r.Shutdown()
	task, outFile := startSh(t, r, "b1", "", "echo hi; exit 3")
	if len(r.Running()) > 1 {
		t.Fatalf("running = %d", len(r.Running()))
	}
	wait(t, task)
	if task.ExitCode != 3 || task.Status() != Failed {
		t.Fatalf("exit %d status %s", task.ExitCode, task.Status())
	}
	data, _ := os.ReadFile(outFile)
	if !strings.Contains(string(data), "hi") || !strings.HasSuffix(string(data), "[exited with code 3]\n") {
		t.Fatalf("output = %q", data)
	}
	if len(r.Running()) != 0 {
		t.Fatal("a finished task is still listed as running")
	}
}

func TestStopOwned_KillsOnlyThatOwnersCommandsAndLeavesAgents(t *testing.T) {
	r := NewRegistry()
	defer r.Shutdown()
	mine, out := startSh(t, r, "mine", "agent-1", "sleep 30")
	other, _ := startSh(t, r, "other", "", "sleep 30")
	agent := NewTask(Agent, "ag")
	agent.Owner = "agent-1"
	r.StartAgent(agent, func(ctx context.Context) { <-ctx.Done() })
	r.EndOfResponse("agent-1")
	if !mine.Finished() || !mine.Killed() || mine.Status() != Stopped {
		t.Fatalf("mine finished=%v killed=%v status=%s", mine.Finished(), mine.Killed(), mine.Status())
	}
	if data, _ := os.ReadFile(out); !strings.HasSuffix(string(data), "[killed]\n") {
		t.Fatalf("output = %q", data)
	}
	if other.Finished() || agent.Finished() {
		t.Fatal("another owner's command, or an agent, was ended")
	}
	if got := r.TakeFinished("agent-1"); len(got) != 0 {
		t.Fatalf("a killed command is handed over: %v", got)
	}
}

func TestReapAtExit_KillsTheMainThreadsCommand(t *testing.T) {
	r := NewRegistry()
	defer r.Shutdown()
	task, _ := startSh(t, r, "b", "", "sleep 30")
	r.ReapAtExit("", 0)
	if !task.Finished() || task.Status() != Stopped {
		t.Fatalf("finished=%v status=%s", task.Finished(), task.Status())
	}
}

func TestTakeFinished_InLaunchOrderAndOnlyOnce(t *testing.T) {
	r := NewRegistry()
	defer r.Shutdown()
	a, _ := startSh(t, r, "a", "o", "true")
	b, _ := startSh(t, r, "b", "o", "true")
	c, _ := startSh(t, r, "c", "other", "true")
	d, _ := startSh(t, r, "d", "o", "sleep 30")
	for _, task := range []*Task{a, b, c} {
		wait(t, task)
	}
	got := r.TakeFinished("o")
	if len(got) != 2 || got[0] != a || got[1] != b {
		t.Fatalf("TakeFinished = %v, want a then b (not c's, not the running d)", got)
	}
	if again := r.TakeFinished("o"); len(again) != 0 {
		t.Fatalf("handed over twice: %v", again)
	}
	if d.Finished() {
		t.Fatal("d finished")
	}
}

func TestAwaitAfterTurn_WaitsForAgentsAndNextTurnSkipsRefused(t *testing.T) {
	r := NewRegistry()
	defer r.Shutdown()
	release := make(chan struct{})
	slow := NewTask(Agent, "slow")
	r.StartAgent(slow, func(context.Context) { <-release })
	fast := NewTask(Agent, "fast")
	r.StartAgent(fast, func(context.Context) {})
	got := make(chan *Task, 1)
	go func() {
		got <- r.NextTurn(context.Background(), "", func(t *Task) bool { return t.ID != "fast" })
	}()
	select {
	case task := <-got:
		t.Fatalf("returned %v while an agent was still running", task)
	case <-time.After(200 * time.Millisecond):
	}
	close(release)
	select {
	case task := <-got:
		if task == nil || task.ID != "slow" {
			t.Fatalf("NextTurn = %v, want slow (fast was refused)", task)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("NextTurn did not return")
	}
	if r.AwaitAfterTurn(context.Background(), "") != nil {
		t.Fatal("nothing is left to wait for")
	}
}

type recorder struct {
	mu    sync.Mutex
	calls []string
}

func (r *recorder) add(c string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, c)
}

func (r *recorder) Changed(running []*Task) {
	ids := []string{}
	for _, t := range running {
		ids = append(ids, t.ID)
	}
	r.add("changed:" + strings.Join(ids, "+"))
}
func (r *recorder) Started(t *Task)           { r.add("started") }
func (r *recorder) Updated(t *Task, s string) { r.add("updated:" + s) }
func (r *recorder) Notified(t *Task)          { r.add("notified") }

func (r *recorder) got() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return strings.Join(r.calls, ",")
}

// startObserved starts a command whose start and end are reported to rec the way a
// harness wires them.
func startObserved(t *testing.T, r *Registry, rec *recorder, id, script string) *Task {
	t.Helper()
	out, err := os.Create(filepath.Join(t.TempDir(), id+".out"))
	if err != nil {
		t.Fatal(err)
	}
	task := NewTask(Command, id)
	if err := r.StartCommand(task, CommandSpec{
		Argv: []string{"/bin/sh", "-c", script}, Dir: t.TempDir(), Env: os.Environ(), Out: out,
		Started: func(t *Task) { Announce(r, t, rec) },
		Ended:   func(t *Task) { Conclude(r, t, rec) },
	}); err != nil {
		t.Fatal(err)
	}
	return task
}

func TestAnnounceAndConclude_TheRunningSetThenTheTask(t *testing.T) {
	r := NewRegistry()
	defer r.Shutdown()
	var rec recorder
	quick := startObserved(t, r, &rec, "q", "sleep 0.3")
	startObserved(t, r, &rec, "l", "sleep 30")
	wait(t, quick)
	if got := rec.got(); got != "changed:q,started,changed:q+l,started,changed:l,updated:completed,notified" {
		t.Fatalf("calls = %s", got)
	}
}

func TestConclude_NamesAKilledCommandAndAForegroundAgentIsNoRunningTask(t *testing.T) {
	r := NewRegistry()
	defer r.Shutdown()
	var rec recorder
	killed := startObserved(t, r, &rec, "k", "sleep 30")
	r.ReapAtExit("", 0)
	wait(t, killed)
	if got := rec.got(); got != "changed:k,started,changed:,updated:killed,notified" {
		t.Fatalf("calls = %s", got)
	}
	// a foreground sub-agent is no registered task: no running set to report
	ok := NewTask(Agent, "ok")
	rec = recorder{}
	Announce(r, ok, &rec)
	Conclude(r, ok, &rec)
	if got := rec.got(); got != "started,updated:completed,notified" {
		t.Fatalf("calls = %s", got)
	}
	rec = recorder{}
	Conclude(nil, ok, &rec)
	if got := rec.got(); got != "updated:completed,notified" {
		t.Fatalf("calls = %s", got)
	}
}

func TestStatus(t *testing.T) {
	cmd := NewTask(Command, "c")
	if cmd.Status() != Completed {
		t.Fatal(cmd.Status())
	}
	cmd.ExitCode = 2
	if cmd.Status() != Failed {
		t.Fatal(cmd.Status())
	}
	ag := NewTask(Agent, "a")
	ag.Failure = "boom"
	if ag.Status() != Failed {
		t.Fatal(ag.Status())
	}
}

func TestShutdown_KillsRunningCommandsAndCancelsAgents(t *testing.T) {
	r := NewRegistry()
	task, _ := startSh(t, r, "s", "", "sleep 30")
	ag := NewTask(Agent, "a")
	r.StartAgent(ag, func(ctx context.Context) { <-ctx.Done() })
	r.Shutdown()
	if !task.Finished() || !ag.Finished() {
		t.Fatalf("command finished=%v agent finished=%v", task.Finished(), ag.Finished())
	}
}

func TestReapAtExit_GraceLetsAQuickCommandFinishAndEndsASlowOne(t *testing.T) {
	r := NewRegistry()
	defer r.Shutdown()
	quick, _ := startSh(t, r, "quick", "", "sleep 0.3")
	slow, _ := startSh(t, r, "slow", "", "sleep 30")
	started := time.Now()
	r.ReapAtExit("", 1500*time.Millisecond)
	if quick.Status() != Completed {
		t.Fatalf("quick = %s: a command that ends within the grace is left to", quick.Status())
	}
	if !slow.Finished() || slow.Status() != Stopped {
		t.Fatalf("slow finished=%v status=%s", slow.Finished(), slow.Status())
	}
	if took := time.Since(started); took < time.Second || took > 8*time.Second {
		t.Fatalf("took %v, want about the grace", took)
	}
}

func TestWaitCeiling(t *testing.T) {
	ctx, cancel := WaitCeiling(context.Background(), 50*time.Millisecond)
	defer cancel()
	select {
	case <-ctx.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("the ceiling did not end the wait")
	}
	none, cancelNone := WaitCeiling(context.Background(), 0)
	defer cancelNone()
	select {
	case <-none.Done():
		t.Fatal("no ceiling ended")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestRunsInBackground(t *testing.T) {
	for _, c := range []struct{ asked, disabled, want bool }{
		{true, false, true},
		{true, true, false},
		{false, false, false},
		{false, true, false},
	} {
		if got := RunsInBackground(c.asked, c.disabled); got != c.want {
			t.Errorf("RunsInBackground(%v, %v) = %v, want %v", c.asked, c.disabled, got, c.want)
		}
	}
}
