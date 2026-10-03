package procexec

import (
	"errors"
	"io"
	"os/exec"
)

// Proc is a child process started in the background.
type Proc struct {
	cmd *exec.Cmd
}

// Start starts the child in its own process group, with its stdout and stderr
// going to out, and returns without waiting: the child outlives the caller's
// turn, and ending it (Kill) reaches whatever it spawned. Its Argv, Dir and Env
// are the Spec's; Stdin, Timeout and Stderr are not used.
func Start(s Spec, out io.Writer) (*Proc, error) {
	if len(s.Argv) == 0 {
		return nil, errors.New("procexec: no program to run")
	}
	cmd := exec.Command(s.Argv[0], s.Argv[1:]...)
	cmd.Dir = s.Dir
	cmd.Env = s.Env
	cmd.Stdout, cmd.Stderr = out, out
	OwnGroup(cmd)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &Proc{cmd: cmd}, nil
}

// Pid is the child's process id.
func (p *Proc) Pid() int { return p.cmd.Process.Pid }

// Wait waits for the child and returns its exit status: 1 when it ended without
// one (it was killed by a signal).
func (p *Proc) Wait() int {
	if err := p.cmd.Wait(); err != nil {
		if p.cmd.ProcessState != nil && p.cmd.ProcessState.ExitCode() >= 0 {
			return p.cmd.ProcessState.ExitCode()
		}
		return 1
	}
	return 0
}

// Kill kills the child's whole process group (see KillGroup).
func (p *Proc) Kill() {
	if p != nil {
		_ = KillGroup(p.cmd)
	}
}
