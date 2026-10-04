package e2e

// notReplaying are the recorded runs whose replay is not green yet, and why.
// The list only shrinks: an entry whose run is gone, or whose run now replays
// green, fails TestGeneratedReplay (except a "flaky:" entry, which is green in
// some runs and so is only checked for its run being there). "adapter:" is
// something of the recording the codex adapter cannot reproduce yet; "mock gap:"
// is behaviour the recording shows that the mock does not have.
var notReplaying = map[string]string{
	"manual-compaction-auto":               "mock gap: the recording compacts on its own when token use passes -c model_auto_compact_token_limit (PostCompact hooks fire); the mock runs no model and counts no tokens, so it never compacts and the replay's hook payloads differ",
	"manual-compaction-auto-blocked":       "mock gap: as manual-compaction-auto (automatic compaction, blocked by a hook); the mock fails the replay: its script emitted the same tool_use 5 turns in a row",
	"manual-compaction-auto-post-stopped":  "mock gap: as manual-compaction-auto (automatic compaction, a PostCompact hook stopping the run); the mock fails the replay: its script emitted the same tool_use 5 turns in a row",
	"noninteractive-run-git-check-refused": "adapter: recorded without --skip-git-repo-check, outside a git repository, with its refusal as the output (exit code and stderr, no event stream); the adapter replays only runs made with the standard flags",
	"noninteractive-run-no-git-check":      "adapter: recorded outside a git repository, without --skip-git-repo-check; the adapter replays only runs made with the standard flags and a repository",
	"noninteractive-run-text-output":       "adapter: recorded without --json, so the output is the final message as text, not an event stream; the adapter compares event streams only",
	"plugin-hooks":                         "adapter: the setup has prepare.sh, which lays out a plugin marketplace and registers it with `codex plugin` before the run; the adapter does not run it",
	"session-fork":                         "adapter: a run of several steps under one CODEX_HOME (then-NN-prompt/args, with the first step's session id); the adapter replays one step",
	"session-resume":                       "adapter: a run of several steps under one CODEX_HOME (then-NN-prompt/args/cwd, with the first step's session id); the adapter replays one step",
	"session-resume-unknown":               "adapter: the run fails before the model is asked (resuming a session that does not exist): there are no model turns to replay, and the adapter compares event streams, not an exit code and stderr",
	"session-start-compact-continue-false": "mock gap: as manual-compaction-auto (automatic compaction after a SessionStart hook's continue:false); the mock fails the replay: its script emitted the same tool_use 5 turns in a row",
	"subagent-stop-block-loop-cap":         "mock gap: the model calls the `wait` function (cell_id, yield_time_ms) to wait on a long-running exec cell, which the mock does not have",
	"subagent-transcripts-v2":              "mock gap: with --enable multi_agent_v2 the model calls spawn_agent and wait_agent as function calls (a task_name, no targets); the mock models the v1 tools only",
	"task-stream-frames":                   "mock gap: the model polls a running command with tools.write_stdin, which the mock does not model (the background-bash cell says so)",
	"session-end-hook-output":              "flaky: its SessionEnd hooks run under the 1 s default timeout the recording shows (runs/session-end*), and a loaded machine (many replays at once) can take longer to start them, so they are killed and log nothing; green when run alone",
}
