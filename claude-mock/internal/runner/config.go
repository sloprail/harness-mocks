package runner

import (
	"io"
	"time"

	"github.com/sloprail/harness-mocks/internal/subagents"
	coretools "github.com/sloprail/harness-mocks/internal/tools"
)

// Config holds the runtime parameters for a mock run.
type Config struct {
	// ScriptPath is the shell script to execute. Its stdout is streamed as JSONL.
	ScriptPath string
	// SessionID is the Claude Code session identifier passed via --resume or --session-id.
	SessionID string
	// AgentID is the sub-agent's id when this Config drives a nested SUB-AGENT run (set by
	// runAgentTool). Empty for the ROOT run. It is stamped onto every PreToolUse payload
	// fired inside this run, so a hook can tell a sub-agent's tool call from the root's —
	// mirroring real claude, where a sub-agent's PreToolUse carries agent_id.
	AgentID string
	// IsResume is true when the caller used --resume (existing session) vs --session-id (new).
	IsResume bool
	// Prompt is the user prompt forwarded to the script via the A10N_MOCK_PROMPT env var.
	Prompt string
	// AdditionalContext is populated from a UserPromptSubmit hook's additionalContext
	// output and forwarded to the script via A10N_MOCK_ADDITIONAL_CONTEXT. This is the
	// real Claude Code mechanism by which a UserPromptSubmit hook augments (never
	// replaces) what the model sees.
	AdditionalContext string
	// Cwd is the working directory for the script and hook invocations.
	Cwd string
	// ProjectDir is used to resolve .claude/settings.json for hook configuration.
	ProjectDir string
	// PluginCacheDir overrides the Claude Code plugin cache root (CLAUDE_CODE_PLUGIN_CACHE_DIR).
	// Marketplaces are cloned/reused under <PluginCacheDir>/<marketplace-slug>/.
	// When empty, falls back to the env var, then /tmp/a10n-mock-plugins.
	// sr:docs https://code.claude.com/docs/en/env-vars#environment-variables (CLAUDE_CODE_PLUGIN_CACHE_DIR)
	PluginCacheDir string
	// ConfigDir overrides the Claude Code global config directory (CLAUDE_CONFIG_DIR).
	// When empty, a temporary directory is created and cleaned up after the run.
	// The session JSONL is written under <ConfigDir>/projects/<encoded-cwd>/<session-id>.jsonl.
	// sr:docs https://code.claude.com/docs/en/agent-sdk/sessions (CLAUDE_CONFIG_DIR)
	ConfigDir string
	// Stderr receives diagnostic output from the mock itself.
	Stderr io.Writer
	// Out receives the passthrough JSONL (defaults to os.Stdout).
	Out io.Writer

	// SuppressSubagentHooks marks a NESTED sub-agent run (set only by the Agent
	// tool, agent.go): no SessionStart, UserPromptSubmit, Stop or SessionEnd of
	// its own — the Agent-tool layer fires SubagentStart/SubagentStop with the
	// sub-agent's agent_id around it, as real Claude Code does.
	// sr:docs https://code.claude.com/docs/en/hooks#subagentstart
	SuppressSubagentHooks bool

	// AgentType is the sub-agent's type when this Config drives a nested SUB-AGENT
	// run. Real Claude Code sends agent_type beside agent_id on every hook event
	// fired inside a sub-agent.
	// sr:docs https://code.claude.com/docs/en/hooks#common-input-fields
	AgentType string

	// SidechainPath is the sub-agent's own transcript
	// (<session>/subagents/agent-<id>.jsonl) for a nested SUB-AGENT run. Real
	// Claude Code writes a sub-agent's records there, never into the parent's
	// file: of 8,119 main transcripts on one machine, zero carry a sidechain
	// record. Empty for the ROOT run.
	SidechainPath string

	// ParentTranscriptPath is the transcript_path the PARENT session reports,
	// carried on every hook payload fired inside a nested sub-agent run — real
	// Claude Code's transcript_path is the session's, with the sub-agent named by
	// agent_id (and its own file by agent_transcript_path, on SubagentStop only).
	ParentTranscriptPath string

	// ForkFrom is the session id --resume named when --fork-session was given:
	// the run continues that conversation under SessionID, in a NEW transcript.
	// See forkTranscript for the shape it writes.
	// sr:docs https://code.claude.com/docs/en/cli-reference#--fork-session
	ForkFrom string

	// PrintMode activates --print mode: the script runs in cfg.Cwd as its working
	// directory, its raw stdout is captured (no JSONL parsing, no session
	// persistence), and written to cfg.Out. Only SessionStart, UserPromptSubmit,
	// and Stop hooks fire. The supervisor (RunSupervisor in services/task-executor) invokes claude with
	// --print and expects:
	//  - cmd.Dir = session dir (the script writes memory files there)
	//  - stdout = raw text (the unclassified user prompt remainder), not JSONL
	//  - no session JSONL written
	//  - SessionStart/UserPromptSubmit/Stop hooks still fire so plugins can intercept
	//
	// sr:docs https://code.claude.com/docs/en/cli-reference#--print
	PrintMode bool

	// Model is the --model the session was started with. The mock runs no
	// model; it only reports the name where real Claude Code reports one (an
	// async Agent receipt's resolvedModel).
	Model string

	// SyncSubagent marks the nested run of a FOREGROUND sub-agent. Real Claude
	// Code kills such a sub-agent's background commands when it gives its final
	// response, and its receipts say so (the 2.1.282 Bash tool's
	// backgroundEndsWithFinalResponse).
	SyncSubagent bool
	Prompting    // what hook payloads tell of the prompt the session is on
	// BgWaitCeiling is how long a `claude -p` run waits idle for background agents
	// after its final turn; zero waits without a limit.
	BgWaitCeiling time.Duration
	// SpawnLimit is how many layers of sub-agents nest below the main thread;
	// 0 is the default.
	SpawnLimit int
	// BackgroundTasksDisabled turns run_in_background off for Bash: the command
	// runs in the foreground. The harness's CLAUDE_CODE_DISABLE_BACKGROUND_TASKS.
	BackgroundTasksDisabled bool

	// TurnLimit is a sub-agent's maxTurns: its nested run ends at it. Nil: none.
	TurnLimit *subagents.TurnLimit

	// bg is the session's background-task registry, shared by the root run
	// and every nested sub-agent run (Stop and SubagentStop list the whole
	// session's tasks). Nil for the root run, which creates it.
	bg *backgroundTasks

	// wake is the session's pending ScheduleWakeup, shared by the root run and
	// every nested run like bg. Nil for the root run, which creates it.
	wake *coretools.Wakeups

	// stream is the session's output stream, shared with every nested run: a
	// sub-agent's task frames go to it, not to the sub-agent's own captured
	// output. It is safe for concurrent writers (a background sub-agent writes
	// while the root does).
	stream io.Writer

	// spawnDepth is how deep in sub-agents this run is: 0 for the root, 1 for
	// a sub-agent it dispatched, 2 for one that sub-agent dispatched.
	spawnDepth int

	// sessionFile is the session's actual transcript, next to which every
	// sub-agent's subagents/agent-<id>.jsonl lives. Set by the root run.
	sessionFile string
}

// projectDirOf is the project root the run's hooks are told: the one given, else
// the working directory.
func projectDirOf(cfg Config) string {
	if cfg.ProjectDir != "" {
		return cfg.ProjectDir
	}
	return cfg.Cwd
}
