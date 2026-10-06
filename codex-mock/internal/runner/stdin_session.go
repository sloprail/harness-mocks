package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/sloprail/harness-mocks/codex-mock/internal/hooks"
	"github.com/sloprail/harness-mocks/internal/toolcall"
)

// stdinTool is Codex's tool that polls (or writes to) a command left running in its own session:
// {session_id, chars?, yield_time_ms, max_output_tokens}. It fires no hook of its own (recorded:
// runs/task-stream-frames, where the hooks saw the spawn, the wait and the polled command, not the poll).
const stdinTool = "write_stdin"

// liveCommand is a command a yielding call left running: the call, the event item it opened, and the file its
// output goes to, read from where the last poll stopped.
type liveCommand struct {
	call     toolcall.Call
	item     string
	cmd      string
	out      string
	read     int64
	finished bool
}

// sessionLog is the run's commands left running, by session id.
type sessionLog struct {
	mu sync.Mutex
	by map[int]*liveCommand
}

func (l *sessionLog) add(id int, s *liveCommand) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.by == nil {
		l.by = map[int]*liveCommand{}
	}
	l.by[id] = s
}

func (l *sessionLog) get(id int) *liveCommand {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.by[id]
}

// pollSession waits for the session's command to end, up to the call's yield time, and tells the agent
// what it printed since the last poll, with its exit code once it has ended, or its session id while it
// has not (recorded: runs/task-stream-frames). When the command has ended its item completes in the
// event stream and its own PostToolUse fires, with what it printed as the response.
func (h toolHost) pollSession(ctx context.Context, c toolcall.Call) toolcall.Result {
	var in struct {
		Session int  `json:"session_id"`
		Yield   *int `json:"yield_time_ms"`
	}
	if err := json.Unmarshal(c.Input, &in); err != nil {
		return toolcall.Result{Output: "write_stdin: invalid input: " + err.Error(), Failed: true}
	}
	s := h.sessions.get(in.Session)
	t := h.bg.Find(strconv.Itoa(in.Session))
	if s == nil || t == nil {
		return toolcall.Result{Output: fmt.Sprintf("write_stdin failed: Unknown process id %d", in.Session), Failed: true}
	}
	yield := 10 * time.Second
	if in.Yield != nil {
		yield = time.Duration(*in.Yield) * time.Millisecond
	}
	began := time.Now()
	ended := false
	select {
	case <-t.Done():
		ended = true
	case <-time.After(yield):
	case <-ctx.Done():
	}
	all, _ := os.ReadFile(s.out)
	printed := string(all[min(int(s.read), len(all)):])
	s.read = int64(len(all))
	reply := map[string]any{"chunk_id": fmt.Sprintf("%06x", rand.Intn(1<<24)), "wall_time_seconds": time.Since(began).Seconds(),
		"original_token_count": (len(printed) + 3) / 4, "output": printed}
	if !ended {
		reply["session_id"] = in.Session
	} else if !s.finished {
		reply["exit_code"] = t.ExitCode
		s.finished = true
		h.events.CommandCompleted(s.item, s.cmd, ttyOutput(s.call, string(all)), t.ExitCode)
		h.fireAfterEnded(ctx, s, string(all))
		os.Remove(s.out)
	}
	b, _ := json.Marshal(reply)
	return toolcall.Result{Output: string(b)}
}

// fireAfterEnded fires the PostToolUse of the call that started a command, now that it has ended.
func (h toolHost) fireAfterEnded(ctx context.Context, s *liveCommand, printed string) {
	own := h.payload(s.call)
	own["tool_response"] = ttyOutput(s.call, printed)
	h.hooks.Fire(ctx, hooks.PostToolUse, fileOrHookName(s.call), own)
}
