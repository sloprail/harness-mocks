package procexec

import (
	"os"
	"os/signal"
	"sync"
	"syscall"
)

// detached are the process groups of commands that returned while a job of theirs still ran
// (Spec.LeaveGroup): they outlive the call, not the mock.
var detached struct {
	mu    sync.Mutex
	pgids map[int]struct{}
}

// keepDetached remembers the group of a command that returned, if anything of it still runs.
func keepDetached(pgid int) {
	if syscall.Kill(-pgid, 0) != nil {
		return
	}
	detached.mu.Lock()
	defer detached.mu.Unlock()
	if detached.pgids == nil {
		detached.pgids = map[int]struct{}{}
	}
	detached.pgids[pgid] = struct{}{}
}

// KillDetached kills every group a LeaveGroup command left running, without waiting for any of
// them. Call it when the mock exits.
func KillDetached() {
	detached.mu.Lock()
	defer detached.mu.Unlock()
	for pgid := range detached.pgids {
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
	}
	detached.pgids = nil
}

// KillDetachedOnSignal kills the groups left running (KillDetached) when the mock is interrupted or
// terminated, then lets the signal take its course.
func KillDetachedOnSignal() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	go func() {
		sig := <-ch
		KillDetached()
		signal.Reset(sig)
		_ = syscall.Kill(os.Getpid(), sig.(syscall.Signal))
	}()
}
