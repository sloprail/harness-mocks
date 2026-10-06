// Package tasks is the harness-neutral core of background tasks: starting them,
// tracking what is running, handing finished ones to their owner, and ending
// them.
package tasks

import (
	"sync/atomic"
	"time"
)

// Kind is what a task is.
type Kind int

const (
	// Command is a shell command running in the background.
	Command Kind = iota
	// Agent is a sub-agent running in the background.
	Agent
)

// Status is how a finished task ended.
type Status string

const (
	Completed Status = "completed"
	Failed    Status = "failed"
	// Stopped is a command ended by the harness (killed), not by itself.
	Stopped Status = "stopped"
)

// Task is one background task. The first fields are its identity and what a
// harness reports of it; the outcome fields are set before the task finishes.
type Task struct {
	ID, ToolUseID string
	// Owner is the agent that launched the task, "" for the main thread: the
	// owner is the one told when the task finishes.
	Owner       string
	Kind        Kind
	Description string
	// Command is a Command task's command line, AgentType an Agent task's type.
	Command, AgentType string
	OutputFile         string
	// Pid is a Command's process id, once started.
	Pid     int
	Started time.Time

	// Meta is what the harness attaches for its own reports of the task (its
	// stream frames); the core never reads it.
	Meta any

	// ExitCode is a Command's exit status (also 1 for a failed Agent).
	ExitCode int
	// Result, Failure, ToolUses and DurationMs are an Agent's outcome.
	Result, Failure string
	ToolUses        int
	DurationMs      int64
	// StoppedAtTurns is the turn limit an Agent stopped at, 0 when it did not.
	StoppedAtTurns int

	done      chan struct{}
	killed    atomic.Bool
	kill      func()
	delivered bool // guarded by Registry.mu
}

// NewTask is a task not yet started.
func NewTask(kind Kind, id string) *Task {
	return &Task{ID: id, Kind: kind, Started: time.Now(), done: make(chan struct{})}
}

// Finished reports whether the task has ended.
func (t *Task) Finished() bool {
	select {
	case <-t.done:
		return true
	default:
		return false
	}
}

// Done is closed when the task ends.
func (t *Task) Done() <-chan struct{} { return t.done }

// Killed reports whether the harness ended the task.
func (t *Task) Killed() bool { return t.killed.Load() }

// Status is how the task ended: stopped when the harness killed it, failed for
// a command with a non-zero exit or an agent that failed, else completed.
func (t *Task) Status() Status {
	switch {
	case t.Killed():
		return Stopped
	case t.Kind == Agent && t.Failure != "":
		return Failed
	case t.Kind == Command && t.ExitCode != 0:
		return Failed
	}
	return Completed
}
