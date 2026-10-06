package e2e

// notReplaying are the recorded runs whose replay is not green yet, and why.
// The list only shrinks: an entry whose run is gone, or whose run now replays
// green, fails TestGeneratedReplay (except a "flaky:" entry, which is green in
// some runs and so is only checked for its run being there). "adapter:" is
// something of the recording the codex adapter cannot reproduce yet;
// "untriaged:" is a replay that differs and has not been looked at (the mock,
// the adapter or the recording may be wrong).
var notReplaying = map[string]string{
	"bg-bash-reaped-at-exit":                     "adapter: an exec_command whose cmd is not a string literal",
	"compaction-transcript-continuity":           "adapter: the setup has args, which the adapter does not install",
	"file-tools":                                 "adapter: the model called tools.apply_patch: the adapter maps exec_command, spawn_agent and wait_agent",
	"file-tools-failure":                         "adapter: the model called tools.apply_patch: the adapter maps exec_command, spawn_agent and wait_agent",
	"hook-command-subdir":                        "adapter: the setup has args, which the adapter does not install",
	"hooks-all-matching-run-same-hook-two-files": "adapter: the setup has project-hooks.json, which the adapter does not install",
	"manual-compaction-auto-blocked":             "adapter: a compaction a hook stopped aborts the turn, which the rollout shows as an interruption the adapter does not turn into a script",
	"manual-compaction-auto-post-stopped":        "adapter: a compaction a hook stopped aborts the turn, which the rollout shows as an interruption the adapter does not turn into a script",
	"nested-session-env":                         "adapter: the setup has env, which the adapter does not install",
	"nested-subagents":                           "adapter: the setup has args, which the adapter does not install",
	"nested-subagents-limit":                     "adapter: the setup has args, which the adapter does not install",
	"nested-subagents-nowait":                    "adapter: the setup has args, which the adapter does not install",
	"noninteractive-run-git-check-refused":       "adapter: recorded with another command line: \"codex exec --json --dangerously-bypass-hook-trust -m gpt-5.6-luna\"",
	"noninteractive-run-no-git-check":            "adapter: recorded with another command line: \"codex exec --json --dangerously-bypass-approvals-and-sandbox --dangerously-bypass-hook-trust -m gpt-5.6-luna\"",
	"noninteractive-run-output-schema":           "adapter: the setup has args, which the adapter does not install",
	"noninteractive-run-text-output":             "adapter: recorded with another command line: \"codex exec --skip-git-repo-check --dangerously-bypass-approvals-and-sandbox --dangerously-bypass-hook-trust -m gpt-5.6-luna\"",
	"plugin-hooks":                               "adapter: the setup has prepare.sh, which the adapter does not install",
	"session-resume-unknown":                     "adapter: the setup has args, which the adapter does not install",
	"subagent-stop-block-loop-cap":               "adapter: the model called wait: the adapter maps only exec",
	"task-stream-frames":                         "adapter: the model called tools.write_stdin: the adapter maps exec_command, spawn_agent and wait_agent",
}
