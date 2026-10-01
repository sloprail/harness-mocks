package runner

import (
	"fmt"
	"strings"

	"github.com/sloprail/harness-mocks/internal/tasks"
)

// backgroundTasks is a session's background tasks: the core registry, which
// decides how they are started, tracked, handed over and ended, plus what is
// Claude Code's about them (their receipts, notifications and stream frames).
type backgroundTasks struct {
	*tasks.Registry
}

func newBackgroundTasks() *backgroundTasks {
	return &backgroundTasks{Registry: tasks.NewRegistry()}
}

// taskNotification is the <task-notification> text for a finished task.
func taskNotification(t *tasks.Task) string {
	var b strings.Builder
	b.WriteString("<task-notification>\n<task-id>" + t.ID + "</task-id>\n")
	if t.ToolUseID != "" {
		b.WriteString("<tool-use-id>" + t.ToolUseID + "</tool-use-id>\n")
	}
	b.WriteString("<output-file>" + t.OutputFile + "</output-file>\n")
	b.WriteString("<status>" + string(t.Status()) + "</status>\n")
	b.WriteString("<summary>" + taskSummary(t) + "</summary>")
	if t.Kind == tasks.Agent {
		b.WriteString("\n<note>" + agentNotificationNote + "</note>")
		if t.Result != "" {
			b.WriteString("\n<result>" + t.Result + "</result>")
		}
		if t.Failure == "" {
			fmt.Fprintf(&b, "\n<usage><subagent_tokens>0</subagent_tokens><tool_uses>%d</tool_uses><duration_ms>%d</duration_ms></usage>", t.ToolUses, t.DurationMs)
		}
	}
	b.WriteString("\n</task-notification>")
	return b.String()
}

// taskSummary is the notification's one-line summary, in the wording of the
// 2.1.282 binary's notification builders.
func taskSummary(t *tasks.Task) string {
	if t.Kind == tasks.Agent {
		if t.Failure != "" {
			return `Agent "` + t.Description + `" failed: ` + t.Failure
		}
		return `Agent "` + t.Description + `" finished`
	}
	if t.ExitCode != 0 {
		return fmt.Sprintf("Background command %q failed with exit code %d", t.Description, t.ExitCode)
	}
	return fmt.Sprintf("Background command %q completed (exit code %d)", t.Description, t.ExitCode)
}
