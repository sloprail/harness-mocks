# claude-mock: evidence for its session-lifecycle behaviour

`claude-mock` stands in for `claude -p`. Each behaviour listed below matches
real Claude Code, and the table gives the evidence for it. Anything without
evidence was removed from the mock. A behaviour that has not been measured yet
is listed under [Not modelled](#not-modelled), not guessed.

## Sources

- **[T] Real transcripts.** Every `~/.claude/projects/**/*.jsonl` on one
  developer machine, scanned 2026-09-27/28: 4,135 main transcripts and 654
  sub-agent transcripts, written by claude 2.1.170–2.1.282. Figures are record
  counts, unless a row says otherwise.
- **[F] Controlled runs of claude 2.1.282, committed as fixtures** in
  [`evidence/`](evidence/). See [`evidence/README.md`](evidence/README.md) for
  how they were made and sanitised. They ran hermetically, under a fake `HOME`,
  so no user or global hooks were loaded. An earlier round of runs did load the
  machine's global `a10n-workspace session stop` Stop hook; that is why those
  runs showed `hookCount: 2`. That round is superseded by these fixtures. The
  runs:

  | Fixture | What it exercises |
  |---|---|
  | `F:bgbash` | A background Bash still running when the turn ends. |
  | `F:midturn` | A background Bash that finishes while the turn is running. |
  | `F:bgagent` | A background Agent still running when the turn ends. |
  | `F:hookerrors` | SessionStart and SubagentStart exit 2, UserPromptSubmit prints plain text, Stop exits 1 with no stderr, SessionEnd prints. |
  | `F:hookmix` | SessionStart and PostToolUse JSON `additionalContext`, SubagentStart and SubagentStop stderr, SubagentStop exit 2. |
  | `F:stops` | A PreToolUse JSON deny, a PreToolUse exit 2, and Stop `decision:block`, then exit 2, then allow. |
  | `F:cap` | A Stop hook that always blocks. |
  | `F:cap-sub` | A SubagentStop hook that always blocks. |
  | `F:compact` | A manual `/compact` with every hook logging. |
  | `F:compact-nohooks` | A manual `/compact` with no hooks. |
  | `F:forkresume` | A session, a `--fork-session` of it, a `--resume` of it, and `--fork-session` without `--resume`. |
  | `F:resume-unknown` | `--resume` of a session that does not exist. |
  | `F:meta` | A foreground, a nested and an isolated sub-agent. |
  | `F:bashfail` | A failing foreground Bash. |
- **[B] The claude 2.1.282 binary.** `strings` of
  `~/.local/share/claude/versions/2.1.282`. Rows quote the minified function or
  template.
- **[D] The hooks reference** (`curl -sL https://code.claude.com/docs/en/hooks.md`,
  fetched 2026-09-27). Rows give the line number.

## Hook payloads

| Behaviour | Evidence |
|---|---|
| Every event carries `transcript_path`. For a sub-agent's events this is the session's path, and the sub-agent is named by `agent_id`/`agent_type`. Main-thread events carry neither field. | D:738. F:bgagent, F:meta: the sub-agents' PreToolUse and SubagentStop payloads. |
| SessionStart `source` is `startup`, `resume`, `compact` or `fork`. A fork is `--resume <id> --fork-session`. | D:1021, D:1138. F:forkresume: `startup`, `fork`, `resume`, `startup`. F:compact: `resume`, then `compact`. |
| `--fork-session` without `--resume` starts a plain session (`startup`) under `--session-id`. | F:forkresume (session …018) |
| A top-level `--resume` fires SessionStart, UserPromptSubmit, Stop and SessionEnd. It fires no SubagentStart and no SubagentStop. | F:forkresume, for both the resume and the fork |
| Stop sends `stop_hook_active` (including `false`), `last_assistant_message`, `background_tasks` and `session_crons`. It sends no `stop_reason`. | D:2536. Every Stop payload in F:bgbash, F:bgagent and F:stops. |
| `stop_hook_active` is true on every Stop after a block. | F:stops: `false`, `true`, `true`. F:cap: `false`, then `true` 8 times. |
| A `background_tasks` entry for a running Bash is `{id, type:"shell", status:"running", description, command}`. For an Agent it is `{id, type:"subagent", status:"running", description, agent_type}`. | F:bgbash, F:bgagent (Stop payloads). D:2536 documents the field but not the entry shape. |
| SubagentStop sends `agent_transcript_path`, `last_assistant_message`, `background_tasks` and `session_crons`. `background_tasks` covers the whole session, and a background sub-agent is listed while it runs, itself included. | D:2385, D:2393. F:bgagent: the SubagentStop payload lists the sub-agent itself. |
| PostToolUse sends `tool_response`, never `tool_output`, plus `duration_ms`. A foreground Bash's response is `{stdout, stderr, interrupted, isImage, noOutputExpected}`. An async Agent's response is its launch result. | D:1986, D:1763. F:bgbash, F:bgagent. |
| A Bash that ran and exited non-zero fires PostToolUseFailure `{tool_name, tool_input, tool_use_id, error: <the text the model got>, is_interrupt, duration_ms}` instead of PostToolUse. A PreToolUse refusal fires neither, although both events had a hook. Other failures (invalid input, a failing Read or Edit) are not measured, so the mock fires neither hook for them. | F:bashfail, F:stops (see each run's `settings.json`) |
| SessionEnd sends `reason`, which is `"other"` for a `-p` run. It sends no `source`. | D:3326. Every SessionEnd payload in the fixtures. |
| PreCompact sends `{trigger, custom_instructions:null}`. PostCompact sends `{trigger, compact_summary}`. | D:3059, D:3085. F:compact. |
| All matching hooks of an event run, even when one of them blocks. | D:414 |
| UserPromptSubmit fires for a task notification that is handed to the agent, with the notification as `prompt`. This happens both mid-turn and when the notification starts a new turn. | F:midturn (mid-turn), F:bgagent (new turn) |

## Hook records in the transcript

| Behaviour | Evidence |
|---|---|
| A hook that exits 0 and prints nothing leaves no record. | T: all 5,713 `hook_success` attachments have non-empty stdout or stderr. F:bgbash: silent hooks leave no attachment. |
| `hook_success {content, stdout, stderr, exitCode, command, durationMs}`: `content` is the plain-text stdout, or `""` when stdout is a JSON object. | T: 5,713 records with exactly this key set. F:hookerrors: UPS `content:"UPS-PLAIN-OUT"`. F:hookmix: `content:""` for JSON. |
| A JSON `additionalContext` adds `hook_additional_context {content:[text]}` directly after that hook's `hook_success`. For SessionStart, `hookName` and `toolUseID` are both `"SessionStart"`. | T: 525 PostToolUse pairs and 4 Stop pairs share a toolUseID. T: all 9 SessionStart contexts are named `SessionStart`. F:hookmix, F:stops. |
| Any exit other than 0 or 2 leaves `hook_non_blocking_error`, with stderr `"Failed with non-blocking status code: <trimmed stderr, or No stderr output>"`. | T: 58 records. F:hookerrors: `No stderr output`. B: `Failed with non-blocking status code: ${stderr.trim()\|\|"No stderr output"}`. |
| An exit 2 is quoted as `"[<command>]: <stderr, or No stderr output>"`. | B: `` `[${command}]: ${stderr\|\|"No stderr output"}` ``. F:hookerrors, F:hookmix, F:stops. |
| SessionStart or SubagentStart exit 2 is a `hook_non_blocking_error`: `stderr` is the quoted form, `exitCode` is 2, and there is no `durationMs`. The session or sub-agent carries on. | D:898. F:hookerrors. |
| SubagentStart records go to the sub-agent's own file. `hookName` is `SubagentStart:<agent_type>`. | D:898. F:hookerrors (exit 2), F:hookmix (hook_success). |
| A Stop or SubagentStop `decision:block` leaves the meta turn `"Stop hook feedback:\n<reason>"`, then `hook_blocking_error {blockingError:{blockingError, command}}`. An empty reason becomes `"Blocked by hook"`. | T: 1,889 of 1,889 `blockingError` objects carry `command`, and every one is preceded by the feedback turn. B: `e.reason\|\|"Blocked by hook"`. F:stops, F:cap. |
| A Stop or SubagentStop exit 2 leaves only the feedback turn, `"Stop hook feedback:\n[<command>]: <stderr>"`. No attachment is written. | F:stops (Stop, main file), F:hookmix (SubagentStop, sub-agent file) |
| A PreToolUse refusal is the tool_result `"PreToolUse:<Tool> hook error: <reason>"` with `is_error:true` and `toolUseResult:"Error: …"`. For exit 2 the reason is the quoted form. For a JSON deny it is `permissionDecisionReason`, then `reason`, then `"Blocked by hook"`. No attachment is written, no PostToolUse or PostToolUseFailure fires, and the turn goes on. | T: 423 such tool_results (355 deny, 68 exit 2), all `is_error`, all with `toolUseResult` `"Error: <content>"`. F:stops. B: `permissionDecisionReason\|\|e.reason\|\|"Blocked by hook"`. |
| PostToolUse exit 2 leaves a `hook_blocking_error` named `PostToolUse:<Tool>`. | B: the PostToolUse hook loop yields `hook_blocking_error` for `blockingError`. |
| A Stop or SubagentStop block continues the turn at most `CLAUDE_CODE_STOP_HOOK_BLOCK_CAP` times in a row (default 8; `0` means no cap). The next block is overridden and the turn ends. For Stop this writes `system/informational {level:"warning", content:"A hook blocked the turn from ending <N> consecutive times — overriding and ending turn. For Stop/SubagentStop hooks, check stop_hook_active in the input and return success while it's true. Set CLAUDE_CODE_STOP_HOOK_BLOCK_CAP to raise this limit."}`, and the result streams `"result":""`. No such record is written for SubagentStop. | D:2536. B: `a.CLAUDE_CODE_STOP_HOOK_BLOCK_CAP??8; if(xe>0&&ve>xe)`, with the message verbatim. F:cap: 9 Stop fires, the warning record, and `"result": ""`. F:cap-sub: 9 SubagentStop fires and no warning record. |
| A turn that Stop re-prompts streams a single `result` frame, at its real end. Each re-run's own result is dropped. | F:stops (2 continuations), F:cap (8): 1 result frame each |
| Every Stop fire that ran a hook ends with `system/stop_hook_summary {hookCount, hookInfos[{command, durationMs}], hookErrors, hookAdditionalContext, preventedContinuation, stopReason, hasOutput, level:"suggestion", toolUseID}`. A blocking hook is listed without `durationMs`. SubagentStop writes no summary. | T: 8,272 records, all in main files. B: `stop_hook_summary` is written when `hookCount>0`. F:hookerrors, F:stops. |
| Only SessionStart, UserPromptSubmit, PreToolUse, PostToolUse, Stop, SubagentStart and SubagentStop leave records. SessionEnd output leaves none. Pre/PostCompact output is display text only (see compaction). WorktreeCreate and WorktreeRemove have no evidence, so they leave none. | F:hookerrors (SessionEnd printed; nothing written). F:compact. B: the PreCompact/PostCompact hooks return `userDisplayMessage`. T: no WorktreeCreate or WorktreeRemove attachment. |
| A PreToolUse or PostToolUse attachment's `toolUseID` is the call's id. For an inline (scenario-written) tool_result it is the `tool_use_id`. | T: 3,560/3,560 PreToolUse and 1,651/1,651 PostToolUse `toolu_…` ids |

## Records and files

| Behaviour | Evidence |
|---|---|
| Every user, assistant, attachment and system record carries `isSidechain`, `userType:"external"`, `entrypoint`, `version`, `cwd`, `sessionId` and `timestamp`. `gitBranch` is added in a git repository. A `-p` run's `entrypoint` is `"sdk-cli"`. | T: 190,464 user/assistant and 85,334 attachment records in main files. Every fixture record has `entrypoint:"sdk-cli"`. B: `gitBranch:await Fl().catch(()=>undefined)` omits the field outside git. |
| A `-p` prompt is marked `promptSource:"sdk"` and `turnOrigin:"sdk"`. | F:midturn, F:bgagent |
| A tool result with empty or whitespace-only content is recorded, and streamed, as `"(<Tool> completed with no output)"`. The structured `toolUseResult` keeps the empty `stdout`. | T: 3,479 of 63,308 Bash results are exactly `(Bash completed with no output)` (1,460 of them carry `toolUseResult.stdout:""`), and 0 are empty. B: `Eyr` checks for empty content and `V` substitutes `` `(${toolName} completed with no output)` ``. F:bashfail. |
| A foreground Bash that exits non-zero is answered `"Exit code N\n<output>"` (`is_error`). Its `toolUseResult` is `"Error: <that text>"`. | T: 1,316 such results (1,239 from 2.1.2xx), every one with that `toolUseResult`. F:bashfail. |
| A sub-agent's records go to `<session>/subagents/agent-<id>.jsonl` with `isSidechain:true` and `agentId`. The first record is the dispatch prompt, parentless. A nested sub-agent's file sits in the same `subagents/` directory. | T: 0 of 4,135 main files hold a sidechain record. F:meta (including the nested sub-agent), F:hookerrors. |
| A finished foreground Agent returns one text block: the `[Subagent hand-back] …The report follows:` frame, then the report with every line indented two spaces (line breaks normalised). An empty report reads `(Subagent completed but returned no output.)`. Next comes the trailer `agentId: <id> (use SendMessage with to: '<id>', summary: '<5-10 word recap>' to continue this agent)[\nworktreePath: <p>]\n<usage>subagent_tokens: N\ntool_uses: N\nduration_ms: N</usage>`. The built-in Explore and Plan agents without a worktree get no trailer. `toolUseResult`, which is also PostToolUse's `tool_response`, is `{status:"completed", prompt, agentId, agentType, harnessNoteCount, harnessTailCount, harnessSectionHash, content, resolvedModel, totalDurationMs, totalTokens, totalToolUseCount}`. | B: the Agent result mapper (`status==="completed"`), `Nnn`/`L$n`/`ODe` (frame and indent), `Lit=new Set(["Explore","Plan"])`, and `DDe` (the hash). F:hookerrors, F:hookmix, F:meta. T: 14 framed results with the trailer and 12 framed Explore/Plan results without it; the 20 older unframed 2.1.2xx results predate the frame. |
| An agent id is `a` followed by 16 hex digits. | T: 618/618 `subagents/agent-*.jsonl` names |
| `.meta.json` is `{agentType, description, toolUseId, spawnDepth, requestShape: "foreground"\|"background", requestNonInteractive: true}`. A nested sub-agent adds `parentAgentId`. A call naming a model adds `model`. An isolated sub-agent adds `worktreePath`, `spawnedWithWorktree: true` and `worktreeBranch: "worktree-agent-<id>"`; the mock creates that branch. | F:meta: foreground depth 1, nested depth 2 with `parentAgentId`. F:bgagent: `background`. T: 626 real sidecars: 210 with `requestShape`, 79 with `parentAgentId`, 80 with `model`, and 3 isolated ones with the worktree fields. |

## Background tasks

| Behaviour | Evidence |
|---|---|
| A background Bash receipt is `"Command running in background with ID: <id>. Output is being written to: <file>. You will be notified when it completes. To check interim output, use Read on that file path."`. When a subcommand is `cd`, `pushd`, `popd` or `chdir`, the line `"\nSession cwd remains <cwd>; directory changes made by the backgrounded command do not apply to subsequent commands."` is added. Its `toolUseResult` is `{stdout, stderr, interrupted, isImage, noOutputExpected, backgroundTaskId[, backgroundCwdHint]}`. | T: 224 receipts (145 without the hint, 79 with it). B: `jIn` builds the receipt, and `A0e`/`Ok` decide the hint. F:bgbash, F:midturn. |
| Inside a foreground sub-agent, the receipt says the command `"is terminated when you give your final response …"` and sets `backgroundEndsWithFinalResponse:true`. The command is killed at the final response. | B: the `backgroundEndsWithFinalResponse` schema doc and the `reapedAtFinalResponse` text |
| Task output is written to `<CLAUDE_CODE_TMPDIR or /tmp>/claude-<uid>/<encoded cwd>/<session>/tasks/<id>.output`, and the file ends with `[exited with code N]` or `[killed]`. | B: `Z_()` and the `tasks` joins. T: 610 receipt paths under `/private/tmp/claude-501/…/tasks/`. B: `tUe` appends the exit line. |
| A background Agent receipt is the exact 6-line template, starting `"Async agent launched successfully. (This tool result is internal metadata …"` and including the `"You know nothing about its results …"`, `"Do not duplicate …"` and `"Do NOT Read or tail …"` lines. It is written as a text-block list, with `toolUseResult {isAsync, status:"async_launched", agentId, description, prompt, outputFile, canReadOutputFile, resolvedModel}`. | T: 671 of 677 receipts match the template byte for byte, and 677 content values are lists. T: 610 `toolUseResult` values with exactly these keys. B: the Agent tool's `async_launched` mapper. F:bgagent. |
| Every sub-agent, foreground or background, has `tasks/<agentId>.output`, a symlink to its JSONL. A background Agent's `output_file` names it. | `evidence/agent-output-files.txt`: 121 of 121 such files under the real HOME are symlinks to the sub-agent's JSONL. The fake-HOME fixtures left 0-byte regular files instead, so the symlink could not be reproduced hermetically. F:meta and F:hookerrors show that foreground sub-agents have the file too: their task frames name it. |
| `description` and `prompt` are required Agent inputs. A call without them is refused with `"<tool_use_error>InputValidationError: Agent failed due to the following issue(s):\nThe required parameter \`x\` is missing</tool_use_error>"`. | B: the Agent zod schema `d({description:o(),prompt:o(),…})`. T: InputValidationError tool_results in this format. |
| A task that finishes while its owner is still working is handed over mid-turn as `attachment {type:"queued_command", prompt:<notification>, source_uuid, commandMode:"task-notification", timestamp}`, after the next tool result. | T: 947 records (762 in main files, 185 in sub-agent files). F:midturn. |
| Stop fires at every end of turn, including while background tasks run. A `-p` session then waits for its background agents. Each finished task starts a new turn with a user record `{origin:{kind:"task-notification"}, promptSource:"system", turnOrigin:"task_notification", queueSkipAttachments:true}`, chained after the `stop_hook_summary`, and Stop fires again. A notification that UserPromptSubmit refuses starts no turn. | T: 824 notification turns, 717 with a stop_hook_summary among the 3 records before them. F:bgagent: Stop, then the notification turn, then Stop. D (UserPromptSubmit decision control) for the refusal. |
| In a `-p` session, a background Bash still running when nothing else is pending is killed. Its `task_notification` stream frame has `status:"stopped"` and `summary:<description>`. Nothing is written to the transcript, and no second turn runs. | F:bgbash |
| Notification text: `<task-notification>` followed by the `task-id`, `tool-use-id`, `output-file`, `status` and `summary` lines. An Agent's notification adds `<note>`, `<result>` (when it has one) and `<usage>` (not on failure). | T: tag sequences, for example 407 with `…summary, note, result, usage`. B: `Mi`/`Det` builders. F:midturn, F:bgagent. |
| Summaries: `Background command "<d>" completed (exit code N)`, `Background command "<d>" failed with exit code N`, `Agent "<d>" finished`, `Agent "<d>" failed: <error>`. The Bash description falls back to the command. | T: 448 `finished`, 72+ `completed (exit code N)`, 4 `failed with exit code N`, 2 `failed:`. B: `eUe` and `Det`. F:midturn (no description given, so the command is used). |
| A `-p` stream-json run streams `system` task frames, each carrying `uuid` and `session_id`:<br>• **Background Bash**: `task_started {task_id, tool_use_id, description, is_backgrounded:true, task_type:"local_bash"}`. When it ends: `task_updated {task_id, patch:{status, end_time}}` (status `completed`, `failed`, or `killed` when stopped at the end), then `task_notification {task_id, tool_use_id, status, output_file, summary}`. The summary is the notification's, or the description when stopped.<br>• **Sub-agent, foreground or background**: `task_started {…, subagent_type, is_backgrounded, spawn_depth, task_type:"local_agent", prompt}`. When it ends: `task_updated`, then `task_notification {…, summary: <the sub-agent's result text>, usage:{total_tokens, tool_uses, duration_ms}}`.<br>• **A background sub-agent's own foreground Bash**: `task_started {…, owned_by_subagent:true, is_backgrounded:false, task_type:"local_bash"}`, then `task_notification {status:"completed", output_file:"", summary:<its description>}`. The status for such a Bash that fails is not measured; the mock writes `failed`. | F:bgbash, F:midturn (Bash). F:bgagent (`summary:"SUBREPLY"`, the owned Bash). F:meta, F:hookerrors, F:cap-sub (foreground sub-agents). F:bashfail: none for the root's own Bash. |
| `TaskOutput` is not a tool. The mock no longer answers it. | T: 0 calls. B: the name is listed only among legacy tool names. |

## Compaction and forks

| Behaviour | Evidence |
|---|---|
| Order of a manual compaction: PreCompact, then SubagentStop for the summarizer, then SessionStart `compact`, then PostCompact. PreCompact exit 2 or `decision:block` stops the compaction. For an automatic compaction the SubagentStop is not measured, so the mock does not fire it. | F:compact: the payload order. D:887 (PreCompact can block). |
| The summarizer's SubagentStop has `agent_type:""`, a fresh `agent_id`, an `agent_transcript_path` to a file that is never written, the summary as `last_assistant_message`, and `stop_hook_active:false`. | F:compact: the payload; no `subagents/` directory exists afterwards. |
| The compact boundary is appended to the current file. It is parentless and has `content:"Conversation compacted"`, `level:"info"` and `isMeta:false`. | T: 46 of 65 boundaries appear part-way down a file. F:compact, F:compact-nohooks. |
| `compactMetadata` has `trigger`, `preTokens`, `durationMs`, `postTokens` and `cumulativeDroppedTokens` (`preTokens − postTokens`, summed over the session's compactions). 2.1.282 always writes `cumulativeDroppedTokens`, and so does the mock; 13 of the 66 real boundaries, all from older versions, lack it. Two shapes have been observed:<br>• **With a preserved segment**: 64 of 65 real boundaries, and F:compact. It adds `preservedSegment{headUuid, anchorUuid, tailUuid}` and `preservedMessages{anchorUuid, uuids, allUuids}`. `anchorUuid` is the summary in 65 of 65. `uuids` are the last N records before the boundary in 31 of 46 real mid-file boundaries; the rest skip some records. N ranges from 2 to 21, and the mock keeps the last N. `allUuids` is `uuids` plus records never written to the file: a strict superset in 44 of 65, and every extra id is unwritten.<br>• **Without one**: F:compact-nohooks and 1 of 65 real boundaries. Neither preserved field is present. The mock writes this shape for `"preserved_segment": false`. | T; F:compact (23138 − 2308 = 20830); F:compact-nohooks (22932 − 2334 = 20598) |
| `logicalParentUuid` is the preserved segment's **tail** (the last id of `uuids`) whenever it names a written record. This holds in all 55 such real boundaries. The tail sits in one of three places:<br>• **The record written immediately before the boundary.** Seen in 34 real boundaries (33 automatic, 1 manual) and in F:compact-nohooks. This is the mock's default.<br>• **An earlier written record**, with the segment ending 2 to 253 records before the boundary. Seen in 7 mid-file boundaries. `"tail_offset": K` reproduces it.<br>• **A record copied in after the boundary.** Seen in the 13 fork files and in 1 mid-file boundary. `forkTranscript` writes this form.<br>The other 11 real boundaries, all automatic, name a record never written, which then closes `allUuids`. The manual F:compact does the same, but it ran under a fake HOME and so is not part of the 66. `"logical_parent":"unwritten"` reproduces this form. Nothing observed predicts which form a compaction takes. Any other `logical_parent` is used as given. sloprail's continuation walk follows this field, so each form is scriptable. | T: 66 boundary records on the machine, re-derived 2026-09-28. These are records, not distinct compactions: a fork file carries a copy of its source's boundary, so one compaction can be counted twice. For example, compaction `d9b3c409…` appears in its original file and in a 2.1.229 fork copy. F:compact (`d99f32a4…`: in `allUuids`, in no record of the file). F:compact-nohooks (a written attachment). |
| The summary (`isCompactSummary`, `isVisibleInTranscriptOnly:true`) chains to the boundary and is written right after it. In 16 of 65 real boundaries some attachments come between them. | T: 66 of 66 summaries carry `isVisibleInTranscriptOnly`. T: after the boundary, 49 summaries and 16 attachments. F:compact. |
| A manual compaction then writes the `/compact` command's three records:<br>• the meta caveat `<local-command-caveat>…</local-command-caveat>`<br>• `<command-name>/compact</command-name>\n            <command-message>compact</command-message>\n            <command-args></command-args>`<br>• `<local-command-stdout>Compacted <lines></local-command-stdout>`, with one `"<Event> [<command>] completed successfully[: <output>]"` line per Pre/PostCompact hook (`… failed …` when it failed), and nothing after `Compacted ` without hooks.<br>SessionStart:compact's attachments come after these records. | F:compact, F:compact-nohooks (`Compacted </local-command-stdout>`). B: `"Compacted "+g.join("\n")` and the Pre/PostCompact message builders. |
| A fork of a compacted session contains: the verbatim boundary, whatever comes before the summary, the summary, the preserved records re-chained after the summary, then the rest. | T: all 19 transcripts that open on a `compact_boundary` |
| A fork of an uncompacted session copies every record with its parents unchanged. `sessionId` is rewritten to the fork's id. | F:forkresume: the fork's copied records carry the fork's `sessionId` and the original parents. T: every 2.1.280+ shared origin carries the file's own id. This contradicts review correction #15, which only 3 files from older 2.1.219/229 support. |
| `--resume <unknown>` prints `"No conversation found with session ID: <id>"` on stderr and, in stream-json mode, a `result/error_during_execution` frame. It exits 1. SessionEnd fires and SessionStart does not. | F:resume-unknown: `stderr.txt`, `stream.jsonl`, `exit.txt`, `payloads.jsonl`. B: the string. |

## Not modelled

These are documented as gaps, not faked:

- `prompt_id`, `permission_mode` and SessionStart `model`/`session_title` on
  payloads.
- A sub-agent's own background work outliving its final response.
- The resume-time notification for a task the previous process left running.
- StopFailure.
- Records left by PostToolUseFailure hooks. The payload is modelled, but there
  is no evidence of what its hooks' output leaves.
- The summarizer SubagentStop of an automatic compaction (not measured).
- Where a summarizer SubagentStop hook's output would be recorded (not
  measured).
- Removing an isolated sub-agent's worktree when it is left clean. Real Claude
  Code then rewrites the sidecar: it drops `worktreePath`,
  `spawnedWithWorktree` and `worktreeBranch`, and adds
  `worktreeCleanlyRemoved: true` (F:meta's `iso` sub-agent; 9 real sidecars).
  The mock keeps the worktree and the fields. **A sloprail consumer reading
  `.meta.json` must expect either shape.**
- The `tasks/<agentId>.output` file of a foreground sub-agent.
- The async Agent receipt's extra line when the sub-agent shares the parent's
  cwd (the binary's `sharesCwd` note). 671 of 677 real receipts have no such
  line, and the mock never adds it.
- Stream frames other than the task frames above: `init`, `task_progress`, `background_tasks_changed`, `notification`, `thinking_tokens`, `commands_changed` and the hook frames. Token counts in `usage` are 0.
- PostToolUseFailure for tool failures other than a Bash exiting non-zero (not measured).
- The foreground Bash frames of a foreground sub-agent (not measured).
- The real `rendered` fields on `queued_command`.
- Separate stdout and stderr for Bash (the mock runs one combined stream).
- Token counts, which are 0.
- A foreground Agent's `usage` and `toolStats` in `toolUseResult`: the mock has
  no API usage or per-category tool counts.
