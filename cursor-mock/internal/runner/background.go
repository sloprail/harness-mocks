package runner

import (
	"context"
	"encoding/json"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sloprail/harness-mocks/cursor-mock/internal/toolexec"
	"github.com/sloprail/harness-mocks/internal/tasks"
)

// Background shells: a Shell call with block_until_ms 0 is left running while
// the agent goes on, answered at once with its shell id and pid (recorded:
// runs/task-notifications-bg, runs/task-notifications-inturn). A
// non-interactive session does not end while one still runs. When one ends, a
// task_notification frame goes on the stream and the agent is given a turn of
// its own to tell the user the result, once the turn it was in has ended:
// Cursor does not hand it over inside that turn, even when the shell ended
// while the agent was still working (a foreground call the shell outlasted by
// seconds, recorded in runs/task-notifications-inturn).

// notificationPrompt is the user turn Cursor starts for a finished shell
// (recorded in the runs' transcripts).
const notificationPrompt = "Briefly inform the user about the task result and perform any follow-up actions (if needed). If there's no follow-ups needed, don't explicitly say that."

// launch starts the shell of a background Shell call and returns its result: the
// shell id and pid, and no output.
func (s *session) launch(c toolexec.Call, useID string, env []string) toolexec.Result {
	id := strconv.Itoa(100000 + rand.Intn(900000))
	folder := s.terminalsFolder()
	_ = os.MkdirAll(folder, 0o755)
	failed := func(err error) toolexec.Result {
		return toolexec.Result{Failed: true, ErrorMessage: err.Error(),
			Frame: map[string]any{"error": map[string]any{"errorMessage": err.Error()}}}
	}
	out, err := os.Create(filepath.Join(folder, id+".txt"))
	if err != nil {
		return failed(err)
	}
	t := tasks.NewTask(tasks.Command, id)
	t.ToolUseID, t.Command, t.Description, t.Owner = useID, c.Command(), c.Description(), s.owner
	if t.Description == "" {
		t.Description = c.Command()
	}
	start := time.Now()
	if err := s.registry().StartCommand(t, tasks.CommandSpec{
		Argv: []string{"/bin/sh", "-c", c.Command()}, Dir: s.cfg.Dir, Env: env, Out: out,
	}); err != nil {
		return failed(err)
	}
	pid := t.Pid
	body := map[string]any{
		"command": c.Command(), "workingDirectory": "", "exitCode": 0, "signal": "", "stdout": "", "stderr": "",
		"executionTime": time.Since(start).Milliseconds(), "shellId": id, "pid": pid,
		"backgroundReason": "SHELL_BACKGROUND_REASON_USER_REQUEST",
	}
	shellID, _ := strconv.Atoi(id)
	output, _ := json.Marshal(map[string]any{"shell_id": shellID, "pid": pid})
	return toolexec.Result{
		Background: true,
		Frame:      map[string]any{"success": body, "isBackground": true, "terminalsFolder": folder},
		ToolOutput: string(output),
		Took:       time.Since(start),
	}
}

// terminalsFolder is where Cursor keeps a project's shells' output, beside its
// transcripts.
func (s *session) terminalsFolder() string {
	project := nonAlnum.ReplaceAllString(strings.TrimPrefix(s.cfg.Dir, "/"), "-")
	return filepath.Join(s.cfg.Home, ".cursor", "projects", project, "terminals")
}

// registries holds each session's background shells (a session is one run, so
// the registry is made on its first background shell, or its first wait).
var registries sync.Map // *session -> *tasks.Registry

func (s *session) registry() *tasks.Registry {
	if s.parent != nil {
		return s.parent.registry()
	}
	r, _ := registries.LoadOrStore(s, tasks.NewRegistry())
	return r.(*tasks.Registry)
}

// afterTurn is what the session does once a turn has ended (the harness side of
// the turn's end): it waits while a background shell still runs, and for each
// one that finished prints its notification and continues the session with the
// turn Cursor gives the agent for it. With none left, the session ends.
//
// sr:provides task-notifications/cursor
func (s *session) afterTurn(ctx context.Context) (string, bool) {
	r := s.registry()
	t := r.AwaitAfterTurn(ctx, "") // a finished shell, at once
	for t == nil {
		running := r.Running()
		if len(running) == 0 || ctx.Err() != nil {
			r.Shutdown()
			registries.Delete(s)
			return "", false
		}
		select { // a shell still runs: the session does not end until it has
		case <-running[0].Done():
		case <-ctx.Done():
		}
		t = r.AwaitAfterTurn(ctx, "")
	}
	s.forward(notificationFrame(s.id, t))
	return notificationPrompt, true
}

// notificationFrame announces a finished shell on the stream.
func notificationFrame(session string, t *tasks.Task) []byte {
	status := "success"
	if t.Status() != tasks.Completed {
		status = "error"
	}
	return jsonLine(map[string]any{
		"type": "system", "subtype": "task_notification", "task_id": t.ID, "status": status,
		"title": t.Description, "session_id": session, "timestamp_ms": time.Now().UnixMilli(),
	})
}
