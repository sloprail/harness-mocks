package runner

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/sloprail/harness-mocks/cursor-mock/internal/toolexec"
)

// awaitTask is a wait on a named background shell: it returns when the shell has
// ended, with the file its output is in, how long it ran and its exit status
// (recorded: runs/task-stream-frames, a wait that ended with the shell, well
// within its limit). A shell the session does not have, one that outlasts the wait,
// and one that did not succeed are not recorded, so each is refused rather than
// answered with a guessed frame.
func (s *session) awaitTask(ctx context.Context, callID string, c toolexec.Call, id string, wait time.Duration) {
	refuse := func(why string) {
		s.forward(errorFrame(s.id, callID, c, s.refuseMsg("cursor-mock: a wait on the task "+strconv.Quote(id)+" "+why), nil))
	}
	t := s.registry().Find(id)
	if t == nil {
		refuse("names a shell this session never started: not modeled")
		return
	}
	select {
	case <-t.Done():
	case <-time.After(wait):
		refuse("outlasted the wait: only a wait that ends with the shell is recorded")
		return
	case <-ctx.Done():
		return
	}
	path := filepath.Join(s.terminalsFolder(), id+".txt")
	size := 0
	if b, err := os.ReadFile(path); err == nil {
		size = len(b)
	}
	if t.ExitCode != 0 || t.Killed() {
		refuse("ended without succeeding: only a shell that succeeded is recorded")
		return
	}
	s.forward(completedFrame(s.id, callID, c, map[string]any{"success": map[string]any{"complete": map[string]any{
		"taskId": id, "runtimeMs": strconv.FormatInt(time.Since(t.Started).Milliseconds(), 10), "outputFilePath": path,
		"outputLength": strconv.Itoa(size), "regexRequested": false, "exitCode": t.ExitCode}}}, nil))
}
