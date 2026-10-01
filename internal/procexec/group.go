package procexec

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

// OwnGroup makes cmd the leader of a new process group, which its children
// inherit, so that stopping the group reaches everything the command spawned.
// Call it before cmd is started.
func OwnGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// KillGroup kills the whole process group of a started cmd that OwnGroup made
// a leader. It reports os.ErrProcessDone when the group is already gone, and
// falls back to killing the command alone only when the group cannot be
// signalled.
func KillGroup(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return os.ErrProcessDone
	}
	// A negative pid is the whole group: signalling the process alone leaves
	// its children running.
	err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	switch {
	case err == nil:
		return nil
	case errors.Is(err, syscall.ESRCH):
		return os.ErrProcessDone
	default:
		return cmd.Process.Kill()
	}
}
