package hooks

import (
	"time"
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

	// recorder, when set, is handed every handler's run — what the harness then
	// writes into the transcript as a hook attachment record.
	recorder func(Input, []HandlerRun)

	agentID, agentType string
}

// SetProjectDir sets the project root every command hook is told as
// CLAUDE_PROJECT_DIR (docs, Reference scripts by path).
// sr:docs https://code.claude.com/docs/en/hooks#reference-scripts-by-path
func (inv *Invoker) SetProjectDir(dir string) { inv.projectDir = dir }

// SetTranscriptPath sets the transcript_path every payload carries unless the
// caller names one.
func (inv *Invoker) SetTranscriptPath(path string) { inv.transcriptPath = path }

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

// HandlerRun is what one hook handler did, in the terms real Claude Code
// records it in a transcript's hook attachment: its command, its streams, its
// exit code and how long it took. Blocked is an exit 2.
type HandlerRun struct {
	Command    string
	Stdout     string
	Stderr     string
	ExitCode   int
	DurationMs int64
	Blocked    bool
	Output     Output
	// JSONParsed: stdout was a JSON object the mock read; on a non-blocking
	// exit status it then decides, not the status (docs, "Other exit codes").
	// JSONError: stdout looked like JSON but did not parse or validate.
	JSONParsed bool
	JSONError  string
	// BlockReason is the blocking reason a blocked handler's JSON gave.
	BlockReason string
	// TimedOut: its timeout cancelled it (TimeoutMs is the limit); its output
	// is discarded.
	TimedOut  bool
	TimeoutMs int64
	// HTTPError: an HTTP hook that failed or answered what cannot be read,
	// a non-blocking error.
	HTTPError string
}

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
	return &Invoker{settings: settings, cwd: cwd, sessionID: sessionID}
}
