package replay

import (
	"bufio"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"syscall"

	core "github.com/sloprail/harness-mocks/internal/replay"
)

// runInterrupted runs the mock as the user interrupted it: SIGINT is sent when its stream shows the
// command the agent ran has started, an event, not a time, and the run is expected to end with
// status 1, as the recorded one did (runs/interrupt-hook).
func runInterrupted(argv []string, dir string, env []string) (string, error) {
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir, cmd.Env = dir, env
	var stderr strings.Builder
	cmd.Stderr = &stderr
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	if err := cmd.Start(); err != nil {
		return "", err
	}
	var out strings.Builder
	sent := false
	sc := bufio.NewScanner(pipe)
	sc.Buffer(make([]byte, 1<<20), 1<<26)
	for sc.Scan() {
		line := sc.Text()
		out.WriteString(line + "\n")
		if !sent && strings.Contains(line, `"command_execution"`) && strings.Contains(line, `"in_progress"`) {
			sent = true
			_ = cmd.Process.Signal(syscall.SIGINT)
		}
	}
	err = cmd.Wait()
	var exit *exec.ExitError
	if !sent || !errors.As(err, &exit) || exit.ExitCode() != 1 {
		return "", &core.MockFailure{Detail: fmt.Sprintf("an interrupted run (interrupted: %v) must end with status 1: %v %s", sent, err, stderr.String())}
	}
	return out.String(), nil
}
