package hooks

import (
	"encoding/json"
	"time"

	corehooks "github.com/sloprail/harness-mocks/internal/hooks"
	"github.com/sloprail/harness-mocks/internal/session"
)

// defaultTimeout is how long a hook with no timeout of its own may run:
// 600 seconds, 30 for a hook that runs before every prompt, and the 1.5-second
// budget a session-end hook shares (docs, "Common fields").
// sr:docs https://code.claude.com/docs/en/hooks#common-fields
func defaultTimeout(ev EventName) time.Duration {
	switch ev {
	case EventUserPromptSubmit:
		return 30 * time.Second
	case EventSessionEnd:
		return 1500 * time.Millisecond
	}
	return 600 * time.Second
}

// Invoker fires hook handlers for a given event and collects their output.
type Invoker struct {
	settings  *Settings
	cwd       string
	sessionID string
	// projectDir is the project root, exported to every command hook as
	// CLAUDE_PROJECT_DIR.
	projectDir string

	// transcriptPath is put on every payload that does not name its own. Real
	// Claude Code sends transcript_path on EVERY hook event (it is one of the
	// documented common input fields), including a sub-agent's tool calls, where
	// it is the SESSION's transcript and the sub-agent is named by agent_id.
	// sr:docs https://code.claude.com/docs/en/hooks#common-input-fields
	transcriptPath string
	scratchpadDir  string
	// configDir is the harness's config directory, which holds the session's env files (CLAUDE_ENV_FILE).
	configDir string
	// sessionTitle is the name of the session a resume found by its name; a UserPromptSubmit carries it.
	sessionTitle string

	// recorder, when set, is handed every handler's run — what the harness then
	// writes into the transcript as a hook attachment record.
	recorder func(Input, []HandlerRun)

	agentID, agentType string

	// turn is the user prompt the session is on, shared with every invoker of a
	// sub-agent run inside it; permissionMode is what the session runs in.
	turn           *corehooks.Turn
	permissionMode string
}

// Turn is the prompt this invoker's events belong to; a sub-agent's invoker
// is given the session's (SetTurn).
func (inv *Invoker) Turn() *Turn { return inv.turn }

// EnsureTurn puts the session on a prompt when it is on none (a compaction).
func (inv *Invoker) EnsureTurn() { inv.turn.Ensure(session.NewID) }

// SetTurn makes the invoker share the prompt state of the session it runs inside.
func (inv *Invoker) SetTurn(t *Turn) { inv.turn = t }

// SetPermissionMode sets the permission_mode the events about a turn carry
// ("bypassPermissions" for a run with --dangerously-skip-permissions).
// sr:docs https://code.claude.com/docs/en/hooks#common-input-fields
func (inv *Invoker) SetPermissionMode(mode string) { inv.permissionMode = mode }

// SetProjectDir sets the project root every command hook is told as
// CLAUDE_PROJECT_DIR (docs, Reference scripts by path).
// sr:docs https://code.claude.com/docs/en/hooks#reference-scripts-by-path
func (inv *Invoker) SetProjectDir(dir string) { inv.projectDir = dir }

// SetTranscriptPath sets the transcript_path every payload carries unless the
// caller names one.
func (inv *Invoker) SetTranscriptPath(path string) { inv.transcriptPath = path }

// Configured is whether any hook is configured for the event.
func (inv *Invoker) Configured(event EventName) bool { return inv.settings.Configured(event) }

// Denied is whether a deny rule of the settings refuses the tool call, and the command it names.
func (inv *Invoker) Denied(tool string, input json.RawMessage) (string, bool) {
	return inv.settings.Denied(tool, input)
}

// SetScratchpadDir sets the scratchpad_dir every payload carries ("" for a session that has none).
func (inv *Invoker) SetScratchpadDir(dir string) { inv.scratchpadDir = dir }

// SetConfigDir sets the config directory a SessionStart hook's CLAUDE_ENV_FILE is under.
func (inv *Invoker) SetConfigDir(dir string) { inv.configDir = dir }

// SetSessionTitle sets the title of the named session this run resumed, which its prompts carry
// (recorded: snapshots/runs/resume-name).
func (inv *Invoker) SetSessionTitle(title string) { inv.sessionTitle = title }

// TranscriptPath is the transcript_path payloads carry by default.
func (inv *Invoker) TranscriptPath() string { return inv.transcriptPath }

// SetAgent makes every payload fired through this invoker carry agent_id and
// agent_type, as real Claude Code does for every hook event fired inside a
// sub-agent. Events that already name an agent keep theirs.
// sr:docs https://code.claude.com/docs/en/hooks#common-input-fields
func (inv *Invoker) SetAgent(agentID, agentType string) {
	inv.agentID, inv.agentType = agentID, agentType
}

// WithRecorder returns a copy of inv that hands its handler runs to fn instead.
func (inv *Invoker) WithRecorder(fn func(Input, []HandlerRun)) *Invoker {
	c := *inv
	c.recorder = fn
	return &c
}

// SetRecorder installs the function every fired event's handler runs are handed
// to, after the handlers have run. See HandlerRun.
func (inv *Invoker) SetRecorder(fn func(Input, []HandlerRun)) { inv.recorder = fn }

// NewInvoker creates an Invoker backed by the given settings. Every command hook
// runs with three Claude-Code environment variables the real CLI sets on each
// session, so a tool the hook shells to sees the same environment it would under
// real claude:
//
//   - CLAUDE_CODE_SESSION_ID — the active session id (from sessionID here), which
//     a tool like `a10n-task-executor session autopilot` reads to resolve "the
//     current session" without an explicit flag. Verified against claude 2.x:
//     SessionStart/UserPromptSubmit/PreToolUse all see it. Set only when non-empty.
//   - CLAUDECODE=1 and CLAUDE_CODE_ENTRYPOINT=sdk-cli — the two variables the real CLI
//     stamps on every session (both confirmed present in a live session). A tool
//     that detects "am I running under a harness" keys off them (sr-agent's harness
//     detection recognises Claude Code by exactly these two and REFUSES with
//     ErrNoHarness when neither is set). The mock STANDS IN FOR Claude Code, so it
//     must present that env unconditionally — see invokeCommand.
//
// sr:docs https://code.claude.com/docs/en/env-vars (CLAUDE_CODE_SESSION_ID, CLAUDECODE, CLAUDE_CODE_ENTRYPOINT)
func NewInvoker(settings *Settings, cwd, sessionID string) *Invoker {
	return &Invoker{settings: settings, cwd: cwd, sessionID: sessionID, turn: &Turn{}}
}
