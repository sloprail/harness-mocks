package e2e

// notReplaying are the recorded runs whose replay is not green yet, and why.
// The list only shrinks: an entry whose run is gone, or whose run now replays
// green, fails TestGeneratedReplay (except a "flaky:" entry, which is green in
// some runs and so is only checked for its run being there). "adapter:" is
// something of the recording the codex adapter cannot reproduce yet;
// "untriaged:" is a replay that differs and has not been looked at (the mock,
// the adapter or the recording may be wrong).
var notReplaying = map[string]string{
	"agent-input-validation":                      "untriaged: the replay differs from the recording (hook payloads: recording 2 lines, mock 1, first difference at line 1)",
	"agent-input-validation-spawn":                "untriaged: the replay differs from the recording (hook payloads: recording 5 lines, mock 3, first difference at line 1)",
	"background-agent":                            "untriaged: the replay differs from the recording (event stream: recording 11 lines, mock 9, first difference at line 6)",
	"bg-bash-reaped-at-exit":                      "adapter: an exec_command whose cmd is not a string literal",
	"compaction-transcript-continuity":            "adapter: the setup has args, which the adapter does not install",
	"file-tools":                                  "adapter: the model called tools.apply_patch: the adapter maps exec_command and spawn_agent",
	"file-tools-failure":                          "adapter: the model called tools.apply_patch: the adapter maps exec_command and spawn_agent",
	"foreground-subagent-bash-ends-with-response": "the mock's spawn_agent waits for the sub-agent and answers with its report; Codex answers at once with {agent_id, nickname} and the model waits with a separate wait_agent call (hooked as multi_agent_v1wait_agent), which the mock does not have. The job's own liveness (the `&` command still running after the sub-agent ended) does replay as recorded",
	"foreground-subagent-result":                  "untriaged: the replay differs from the recording (hook payloads: recording 3 lines, mock 2, first difference at line 1)",
	"hook-command-subdir":                         "adapter: the setup has args, which the adapter does not install",
	"hook-exit-codes":                             "untriaged: the replay differs from the recording (event stream: recording 11 lines, mock 11, first difference at line 9)",
	"hooks-all-matching-run-same-hook-two-files":  "adapter: the setup has project-hooks.json, which the adapter does not install",
	"manual-compaction-auto":                      "adapter: the setup has args, which the adapter does not install",
	"manual-compaction-auto-blocked":              "adapter: the setup has args, which the adapter does not install",
	"manual-compaction-auto-post-stopped":         "adapter: the setup has args, which the adapter does not install",
	"nested-session-env":                          "adapter: the setup has env, which the adapter does not install",
	"nested-subagents":                            "adapter: the setup has args, which the adapter does not install",
	"nested-subagents-limit":                      "adapter: the setup has args, which the adapter does not install",
	"nested-subagents-nowait":                     "adapter: the setup has args, which the adapter does not install",
	"noninteractive-run-git-check-refused":        "adapter: recorded with another command line: \"codex exec --json --dangerously-bypass-hook-trust -m gpt-5.6-luna\"",
	"noninteractive-run-no-git-check":             "adapter: recorded with another command line: \"codex exec --json --dangerously-bypass-approvals-and-sandbox --dangerously-bypass-hook-trust -m gpt-5.6-luna\"",
	"noninteractive-run-output-schema":            "adapter: the setup has args, which the adapter does not install",
	"noninteractive-run-text-output":              "adapter: recorded with another command line: \"codex exec --skip-git-repo-check --dangerously-bypass-approvals-and-sandbox --dangerously-bypass-hook-trust -m gpt-5.6-luna\"",
	"plugin-hooks":                                "adapter: the setup has prepare.sh, which the adapter does not install",
	"print-waits-for-background-agents":           "untriaged: the replay differs from the recording (event stream: recording 9 lines, mock 11, first difference at line 8)",
	"session-end-hook-failure":                    "untriaged: the replay differs from the recording (event stream: recording 9 lines, mock 9, first difference at line 4)",
	"session-end-hook-output":                     "flaky: untriaged: the mock logs no SessionEnd payload in some runs (6 recorded, 0 logged), green in others",
	"session-fork":                                "adapter: the setup has then-01-args, which the adapter does not install",
	"session-resume":                              "adapter: the setup has then-01-args, which the adapter does not install",
	"session-resume-unknown":                      "adapter: the setup has args, which the adapter does not install",
	"session-start-compact-continue-false":        "adapter: the setup has args, which the adapter does not install",
	"stops":                                       "untriaged: the replay differs from the recording (event stream: recording 11 lines, mock 11, first difference at line 8)",
	"subagent-stop-block-loop-cap":                "adapter: the model called wait: the adapter maps only exec",
	"subagent-transcripts-v2":                     "adapter: the setup has args, which the adapter does not install",
	"subagent-worktree-isolation":                 "untriaged: the replay differs from the recording (hook payloads: recording 4 lines, mock 3, first difference at line 1)",
	"subprocess-session-env":                      "untriaged: the replay differs from the recording (event stream: recording 8 lines, mock 8, first difference at line 5)",
	"task-stream-frames":                          "adapter: the model called tools.write_stdin: the adapter maps exec_command and spawn_agent",
}
