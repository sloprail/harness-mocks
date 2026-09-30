package runner

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type backgroundTask struct {
	id          string
	toolUseID   string
	owner       string // agent id of the launcher; "" for the main thread
	agent       bool
	description string
	command     string // a Bash task's command
	agentType   string // an Agent task's type
	outputFile  string
	started     time.Time

	done     chan struct{}
	exitCode int
	killed   atomic.Bool
	// an Agent task's outcome
	result     string
	failure    string
	toolUses   int
	durationMs int64

	cmd *exec.Cmd

	delivered bool // guarded by backgroundTasks.mu
}

func (t *backgroundTask) finished() bool {
	select {
	case <-t.done:
		return true
	default:
		return false
	}
}

type backgroundTasks struct {
	mu      sync.Mutex
	tasks   []*backgroundTask
	changed chan struct{} // signalled (non-blocking) whenever a task finishes
	wg      sync.WaitGroup
	cancel  context.CancelFunc
	ctx     context.Context
}

func newBackgroundTasks() *backgroundTasks {
	ctx, cancel := context.WithCancel(context.Background())
	return &backgroundTasks{changed: make(chan struct{}, 1), ctx: ctx, cancel: cancel}
}

func (b *backgroundTasks) add(t *backgroundTask) {
	b.mu.Lock()
	b.tasks = append(b.tasks, t)
	b.mu.Unlock()
}

func (b *backgroundTasks) finish(t *backgroundTask) {
	close(t.done)
	select {
	case b.changed <- struct{}{}:
	default:
	}
}

// notification is the <task-notification> text for a finished task.
func (t *backgroundTask) notification() string {
	var b strings.Builder
	b.WriteString("<task-notification>\n<task-id>" + t.id + "</task-id>\n")
	if t.toolUseID != "" {
		b.WriteString("<tool-use-id>" + t.toolUseID + "</tool-use-id>\n")
	}
	b.WriteString("<output-file>" + t.outputFile + "</output-file>\n")
	b.WriteString("<status>" + t.status() + "</status>\n")
	b.WriteString("<summary>" + t.summary() + "</summary>")
	if t.agent {
		b.WriteString("\n<note>" + agentNotificationNote + "</note>")
		if t.result != "" {
			b.WriteString("\n<result>" + t.result + "</result>")
		}
		if t.failure == "" {
			fmt.Fprintf(&b, "\n<usage><subagent_tokens>0</subagent_tokens><tool_uses>%d</tool_uses><duration_ms>%d</duration_ms></usage>", t.toolUses, t.durationMs)
		}
	}
	b.WriteString("\n</task-notification>")
	return b.String()
}

func (t *backgroundTask) status() string {
	switch {
	case t.killed.Load():
		return "stopped"
	case t.agent && t.failure != "":
		return "failed"
	case !t.agent && t.exitCode != 0:
		return "failed"
	}
	return "completed"
}

// summary is the notification's one-line summary, in the wording of the
// 2.1.282 binary's notification builders.
func (t *backgroundTask) summary() string {
	if t.agent {
		if t.failure != "" {
			return `Agent "` + t.description + `" failed: ` + t.failure
		}
		return `Agent "` + t.description + `" finished`
	}
	if t.exitCode != 0 {
		return fmt.Sprintf("Background command %q failed with exit code %d", t.description, t.exitCode)
	}
	return fmt.Sprintf("Background command %q completed (exit code %d)", t.description, t.exitCode)
}
