package runner

import (
	"errors"
	"os/exec"
	"syscall"
)

// stopOwned ends what owner still has running when its run gives its final
// response: background commands are killed, and each killed one's stopped
// notification goes to the stream only (a `claude -p` session writes none to
// the transcript). It waits for them to be gone.
func (b *backgroundTasks) stopOwned(cfg Config) {
	if b == nil {
		return
	}
	b.mu.Lock()
	var victims []*backgroundTask
	for _, t := range b.tasks {
		if t.owner == cfg.AgentID && !t.agent && !t.finished() {
			t.killed.Store(true)
			t.delivered = true
			victims = append(victims, t)
		}
	}
	b.mu.Unlock()
	for _, t := range victims {
		killGroup(t.cmd)
		<-t.done
	}
}

// shutdown ends the whole session's background work when the run returns: a
// mock run is the session, and nothing outlives it. It kills every command's
// process group, cancels running sub-agents, and waits for all of it.
func (b *backgroundTasks) shutdown() {
	if b == nil {
		return
	}
	b.mu.Lock()
	var cmds []*exec.Cmd
	for _, t := range b.tasks {
		if !t.agent && !t.finished() {
			t.killed.Store(true)
			cmds = append(cmds, t.cmd)
		}
	}
	b.mu.Unlock()
	for _, c := range cmds {
		killGroup(c)
	}
	b.cancel()
	b.wg.Wait()
}

// killGroup kills a background command's whole process group.
func killGroup(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		_ = cmd.Process.Kill()
	}
}
