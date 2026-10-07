package procexec

import (
	"context"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

func sh(script string) Spec {
	return Spec{Argv: []string{"/bin/sh", "-c", script}, Env: []string{"PATH=/usr/bin:/bin", "X=1"}}
}

func TestRunReportsOutputStdinAndExit(t *testing.T) {
	s := sh(`read l; echo "got $l $X"; echo err >&2; exit 3`)
	s.Stdin = []byte("line\n")
	res, err := Run(context.Background(), s)
	if err != nil || !res.Started {
		t.Fatalf("Run = %+v, %v", res, err)
	}
	if res.ExitCode != 3 || string(res.Stdout) != "got line 1\n" || string(res.Stderr) != "err\n" {
		t.Fatalf("result = %+v", res)
	}
}

func TestRunUnstartableProgramIsAnError(t *testing.T) {
	res, err := Run(context.Background(), Spec{Argv: []string{"/nonexistent/program"}})
	if err == nil || res.Started || res.ExitCode != -1 {
		t.Fatalf("Run = %+v, %v; want an unstarted error", res, err)
	}
}

func TestRunTimeoutKillsTheWholeGroup(t *testing.T) {
	s := sh(`sleep 30 & echo $!; wait`)
	s.Timeout = 300 * time.Millisecond
	start := time.Now()
	res, err := Run(context.Background(), s)
	if err != nil || !res.TimedOut || res.ExitCode != -1 {
		t.Fatalf("Run = %+v, %v; want a timeout", res, err)
	}
	if time.Since(start) > 10*time.Second {
		t.Fatalf("the timeout took %v", time.Since(start))
	}
	pid := strings.TrimSpace(string(res.Stdout))
	probe, _ := Run(context.Background(), sh(`kill -0 `+pid+` 2>/dev/null && echo alive || echo gone`))
	if got := strings.TrimSpace(string(probe.Stdout)); got != "gone" {
		t.Fatalf("grandchild %s is %s after the timeout", pid, got)
	}
}

// Stdout is given what the child writes as it writes it, and OnStart a way to signal the child, so
// a caller can act on what the output shows: here SIGTERM once "ready" is printed.
func TestStdoutAndOnStartLetACallerSignalTheChildWhenItsOutputShowsSomething(t *testing.T) {
	started := make(chan func(os.Signal) error, 1)
	var once sync.Once
	w := writerFunc(func(p []byte) {
		if strings.Contains(string(p), "ready") {
			once.Do(func() { _ = (<-started)(syscall.SIGTERM) }) // OnStart ran before the child could print anything we wait on
		}
	})
	res, err := Run(context.Background(), Spec{
		Argv: []string{"/bin/sh", "-c", "sleep 0.3; echo ready; sleep 30"}, Stdout: w,
		OnStart: func(s func(os.Signal) error) { started <- s },
	})
	if err != nil || !res.Started {
		t.Fatalf("Run: %v %+v", err, res)
	}
	if !strings.Contains(string(res.Stdout), "ready") || res.ExitCode == 0 {
		t.Fatalf("result = %+v", res)
	}
}

type writerFunc func([]byte)

func (f writerFunc) Write(p []byte) (int, error) { f(p); return len(p), nil }

// A LeaveGroup command's background job outlives the call, and KillDetached ends it.
func TestRunLeaveGroup_JobOutlivesTheCallUntilKillDetached(t *testing.T) {
	res, err := Run(context.Background(), Spec{Argv: []string{"/bin/sh", "-c", "sleep 30 >/dev/null 2>&1 & echo $!"}, LeaveGroup: true})
	if err != nil || res.ExitCode != 0 {
		t.Fatalf("run: %v %+v", err, res)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(res.Stdout)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGKILL) })
	if syscall.Kill(pid, 0) != nil {
		t.Fatal("the job was ended with the call")
	}
	KillDetached()
	deadline := time.Now().Add(3 * time.Second)
	for syscall.Kill(pid, 0) == nil && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if syscall.Kill(pid, 0) == nil {
		t.Fatal("KillDetached left the job running")
	}
}
