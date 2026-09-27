# claude-mock: evidence for its session-lifecycle behaviour

`claude-mock` stands in for `claude -p`. Each behaviour listed below matches
real Claude Code, and the table gives the evidence for it. Anything without
evidence was removed from the mock. A behaviour that has not been measured yet
is listed under [Not modelled](#not-modelled), not guessed.

## Sources

- **[T] Real transcripts.** Every `~/.claude/projects/**/*.jsonl` on one
  developer machine, scanned on 2026-09-27: 4,135 main transcripts and 654
  sub-agent transcripts, written by claude 2.1.170–2.1.282. Figures are record
  counts, unless a row says otherwise.
- **[R] Controlled runs of claude 2.1.282.** `claude -p --model haiku
  --dangerously-skip-permissions --output-format stream-json --verbose`, run in
  scratch git repositories. Each run used a project `.claude/settings.json`
  whose hooks logged their stdin payloads and, where the run needed it, printed
  output or exited non-zero. The runs:
  - R1: a background Bash (`sleep 8`) still running when the turn ends.
  - R2: a background Bash (`echo`) that finishes while the turn is running.
  - R3: a background Agent still running when the turn ends.
  - R4: SessionStart and SubagentStart exit 2, UserPromptSubmit prints plain
    text, Stop exits 1 with no stderr, SessionEnd prints to stdout and stderr.
  - R5: PreToolUse exit 2, PostToolUse JSON `additionalContext`, SubagentStart
    and SubagentStop print to stderr.
  - R6: SessionStart JSON `additionalContext`, a PreToolUse JSON deny, Stop
    `decision:block` then exit 2 then allow, SubagentStop exit 2.
  - R7: `/compact` on the R6 session, with PreCompact and PostCompact hooks.
  - R8: `--resume <unknown>`, `--fork-session` without `--resume`, a fork of R1,
    and a plain resume of R3.
- **[B] The claude 2.1.282 binary.** `strings` of
  `~/.local/share/claude/versions/2.1.282`. Rows quote the minified function or
  template.
- **[D] The hooks reference** (`curl -sL https://code.claude.com/docs/en/hooks.md`,
  fetched 2026-09-27). Rows give the line number.

## Hook payloads

| Behaviour | Evidence |
|---|---|
| Every event carries `transcript_path`. For a sub-agent's events this is the session's path, and the sub-agent is named by `agent_id`/`agent_type`. Main-thread events carry neither field. | D:738, R3 (the sub-agent's PreToolUse and SubagentStop) |
| SessionStart `source` is `startup`, `resume`, `compact` or `fork`. A fork is `--resume <id> --fork-session`. | D:1021, D:1138. R8: the fork payload has `"source":"fork"`, and the resume payload has `"resume"`. |
| `--fork-session` without `--resume` starts a plain session (`startup`) under `--session-id`. | R8 |
| A top-level `--resume` fires SessionStart, UserPromptSubmit, Stop and SessionEnd. It fires no SubagentStart and no SubagentStop. | R8 (resume of R3). Also R8 (fork). |
| Stop sends `stop_hook_active` (including `false`), `last_assistant_message`, `background_tasks` and `session_crons`. It sends no `stop_reason`. | D:2536. The R1/R3/R6 payloads. |
| `stop_hook_active` is true on every Stop after a block. | R6: `false`, `true`, `true`. |
| A `background_tasks` entry for a running Bash is `{id, type:"shell", status:"running", description, command}`. For an Agent it is `{id, type:"subagent", status:"running", description, agent_type}`. | R1, R3 |
| SubagentStop sends `agent_transcript_path`, `last_assistant_message`, `background_tasks` and `session_crons`. `background_tasks` covers the whole session, and the sub-agent's own task is listed while it runs. | D:2385, D:2393. R3: the SubagentStop payload lists itself. |
| PostToolUse sends `tool_response`, never `tool_output`. A foreground Bash's response is `{stdout, stderr, interrupted, isImage, noOutputExpected}`. An async Agent's response is its launch result. | D:1986, D:1763. R1: `tool_response` of the background Bash. |
| SessionEnd sends `reason`, which is `"other"` for a `-p` run. It sends no `source`. | D:3326. R1–R8: every SessionEnd payload has `"reason":"other"`. |
| PreCompact sends `{trigger, custom_instructions:null}`. PostCompact sends `{trigger, compact_summary}`. | D:3059, D:3085. R7: `"trigger":"manual"` and `custom_instructions` null. |
| All matching hooks of an event run, even when one of them blocks. | D:414 |
| UserPromptSubmit fires for a task notification that is handed to the agent, with the notification as `prompt`. This happens both mid-turn and when the notification starts a new turn. | R2 (mid-turn), R3 (new turn) |

## Hook records in the transcript

| Behaviour | Evidence |
|---|---|
| A hook that exits 0 and prints nothing leaves no record. | T: all 5,713 `hook_success` attachments have non-empty stdout or stderr. R1: silent hooks leave no attachment. |
| `hook_success {content, stdout, stderr, exitCode, command, durationMs}`: `content` is the plain-text stdout, or `""` when stdout is a JSON object. | T: 5,713 records with exactly this key set. R4: UPS `content:"UPS-PLAIN-OUT"`. R5, R6: `content:""` for JSON. |
| A JSON `additionalContext` adds `hook_additional_context {content:[text]}` directly after that hook's `hook_success`. For SessionStart, `hookName` and `toolUseID` are both `"SessionStart"`. | T: 525 PostToolUse pairs and 4 Stop pairs share a toolUseID. T: all 9 SessionStart contexts are named `SessionStart`. R5, R6. |
| Any exit other than 0 or 2 leaves `hook_non_blocking_error`, with stderr `"Failed with non-blocking status code: <trimmed stderr, or No stderr output>"`. | T: 58 records. R4: `No stderr output`. B: `Failed with non-blocking status code: ${stderr.trim()\|\|"No stderr output"}`. |
| An exit 2 is quoted as `"[<command>]: <stderr, or No stderr output>"`. | B: `` `[${command}]: ${stderr\|\|"No stderr output"}` ``. R4, R5, R6. |
| SessionStart or SubagentStart exit 2 is a `hook_non_blocking_error`: `stderr` is the quoted form, `exitCode` is 2, and there is no `durationMs`. The session or sub-agent carries on. | D:898. R4. |
| SubagentStart records go to the sub-agent's own file. `hookName` is `SubagentStart:<agent_type>`. | D:898. R4 (exit 2), R5 (hook_success). |
| A Stop or SubagentStop `decision:block` leaves the meta turn `"Stop hook feedback:\n<reason>"`, then `hook_blocking_error {blockingError:{blockingError, command}}`. An empty reason becomes `"Blocked by hook"`. | T: 1,889 of 1,889 `blockingError` objects carry `command`, and every one is preceded by the feedback turn. B: `e.reason\|\|"Blocked by hook"`. R6. |
| A Stop or SubagentStop exit 2 leaves only the feedback turn, `"Stop hook feedback:\n[<command>]: <stderr>"`. No attachment is written. | R6: the main file (Stop) and the sub-agent file (SubagentStop). |
| A PreToolUse refusal is the tool_result `"PreToolUse:<Tool> hook error: <reason>"` with `is_error:true` and `toolUseResult:"Error: …"`. For exit 2 the reason is the quoted form. For a JSON deny it is `permissionDecisionReason`, then `reason`, then `"Blocked by hook"`. No attachment is written, no PostToolUse fires, and the turn goes on. | T: 67 such tool_results, all `is_error`. R5 (exit 2), R6 (deny). B: `permissionDecisionReason\|\|e.reason\|\|"Blocked by hook"`. |
| PostToolUse exit 2 leaves a `hook_blocking_error` named `PostToolUse:<Tool>`. | B: the PostToolUse hook loop yields `hook_blocking_error` for `blockingError`. |
| Every Stop fire that ran a hook ends with `system/stop_hook_summary {hookCount, hookInfos[{command, durationMs}], hookErrors, hookAdditionalContext, preventedContinuation, stopReason, hasOutput, level:"suggestion", toolUseID}`. A blocking hook is listed without `durationMs`. SubagentStop writes no summary. | T: 8,272 records, all in main files. B: `stop_hook_summary` is written when `hookCount>0`. R4, R6. |
| Only SessionStart, UserPromptSubmit, PreToolUse, PostToolUse, Stop, SubagentStart and SubagentStop leave records. SessionEnd output leaves none. Pre/PostCompact output is display text only. WorktreeCreate and WorktreeRemove have no evidence, so they leave none. | R4 (SessionEnd), R7 (Pre/PostCompact), B (the PreCompact/PostCompact hooks return `userDisplayMessage`). T: no WorktreeCreate or WorktreeRemove attachment. |
| A PreToolUse or PostToolUse attachment's `toolUseID` is the call's id. For an inline (scenario-written) tool_result it is the `tool_use_id`. | T: 3,560/3,560 PreToolUse and 1,651/1,651 PostToolUse `toolu_…` ids. |

## Records and files

| Behaviour | Evidence |
|---|---|
| Every user, assistant, attachment and system record carries `isSidechain`, `userType:"external"`, `entrypoint`, `version`, `cwd`, `sessionId` and `timestamp`. `gitBranch` is added in a git repository. A `-p` run's `entrypoint` is `"sdk-cli"`. | T: 190,464 user/assistant and 85,334 attachment records in main files. R1–R8 records have `entrypoint:"sdk-cli"`. B: `gitBranch:await Fl().catch(()=>undefined)` omits the field outside git. |
| A `-p` prompt is marked `promptSource:"sdk"` and `turnOrigin:"sdk"`. | R2, R3 |
| A sub-agent's records go to `<session>/subagents/agent-<id>.jsonl` with `isSidechain:true` and `agentId`. The first record is the dispatch prompt, parentless. A nested sub-agent's file sits in the same `subagents/` directory. | T: 0 of 4,135 main files hold a sidechain record. R3, R4, R5. |
| An agent id is `a` followed by 16 hex digits. | T: 618/618 `subagents/agent-*.jsonl` names |
| `.meta.json` holds `{agentType, description, toolUseId}`, plus `worktreePath` only for an isolated sub-agent. | R3: `{"agentType","description","toolUseId",…}` with no `worktreePath` |

## Background tasks

| Behaviour | Evidence |
|---|---|
| A background Bash receipt is `"Command running in background with ID: <id>. Output is being written to: <file>. You will be notified when it completes. To check interim output, use Read on that file path."`. When a subcommand is `cd`, `pushd`, `popd` or `chdir`, the line `"\nSession cwd remains <cwd>; directory changes made by the backgrounded command do not apply to subsequent commands."` is added. Its `toolUseResult` is `{stdout, stderr, interrupted, isImage, noOutputExpected, backgroundTaskId[, backgroundCwdHint]}`. | T: 224 receipts (145 without the hint, 79 with it). B: `jIn` builds the receipt, and `A0e`/`Ok` decide the hint. |
| Inside a foreground sub-agent, the receipt says the command `"is terminated when you give your final response …"` and sets `backgroundEndsWithFinalResponse:true`. The command is killed at the final response. | B: the `backgroundEndsWithFinalResponse` schema doc and the `reapedAtFinalResponse` text |
| Task output is written to `<CLAUDE_CODE_TMPDIR or /tmp>/claude-<uid>/<encoded cwd>/<session>/tasks/<id>.output`, and the file ends with `[exited with code N]` or `[killed]`. | B: `Z_()` and the `tasks` joins. T: 610 receipt paths under `/private/tmp/claude-501/…/tasks/`. B: `tUe` appends the exit line. |
| A background Agent receipt is the exact 6-line template, starting `"Async agent launched successfully. (This tool result is internal metadata …"` and including the `"You know nothing about its results …"`, `"Do not duplicate …"` and `"Do NOT Read or tail …"` lines. It is written as a text-block list, with `toolUseResult {isAsync, status:"async_launched", agentId, description, prompt, outputFile, canReadOutputFile, resolvedModel}`. | T: 671 of 677 receipts match the template byte for byte, and 677 content values are lists. T: 610 `toolUseResult` values with exactly these keys. B: the Agent tool's `async_launched` mapper. |
| An Agent's `output_file` is `tasks/<agentId>.output`, a symlink to the sub-agent's JSONL. | R3 (`ls -la`), and the machine's live task directories |
| `description` and `prompt` are required Agent inputs. A call without them is refused with `"<tool_use_error>InputValidationError: Agent failed due to the following issue(s):\nThe required parameter \`x\` is missing</tool_use_error>"`. | B: the Agent zod schema `d({description:o(),prompt:o(),…})`. T: InputValidationError tool_results in this format. |
| A task that finishes while its owner is still working is handed over mid-turn as `attachment {type:"queued_command", prompt:<notification>, source_uuid, commandMode:"task-notification", timestamp}`, after the next tool result. | T: 947 records (762 in main files, 185 in sub-agent files). R2, including a `-p` run. |
| Stop fires at every end of turn, including while background tasks run. A `-p` session then waits for its background agents. Each finished task starts a new turn with a user record `{origin:{kind:"task-notification"}, promptSource:"system", turnOrigin:"task_notification", queueSkipAttachments:true}`, chained after the `stop_hook_summary`, and Stop fires again. | T: 824 notification turns, 717 with a stop_hook_summary among the 3 records before them. R3: Stop, then the notification turn, then Stop. |
| In a `-p` session, a background Bash still running when nothing else is pending is killed. Its `task_notification` stream frame has `status:"stopped"` and `summary:<description>`. Nothing is written to the transcript, and no second turn runs. | R1 |
| Notification text: `<task-notification>` followed by the `task-id`, `tool-use-id`, `output-file`, `status` and `summary` lines. An Agent's notification adds `<note>`, `<result>` (when it has one) and `<usage>` (not on failure). | T: tag sequences, for example 407 with `…summary, note, result, usage`. B: `Mi`/`Det` builders. R2, R3. |
| Summaries: `Background command "<d>" completed (exit code N)`, `Background command "<d>" failed with exit code N`, `Agent "<d>" finished`, `Agent "<d>" failed: <error>`. The Bash description falls back to the command. | T: 448 `finished`, 72+ `completed (exit code N)`, 4 `failed with exit code N`, 2 `failed:`. B: `eUe` and `Det`. R2 (no description given, so the command is used). |
| `TaskOutput` is not a tool. The mock no longer answers it. | T: 0 calls. B: the name is listed only among legacy tool names. |

## Compaction and forks

| Behaviour | Evidence |
|---|---|
| Order: PreCompact, then the boundary and summary, then SessionStart `compact`, then PostCompact. PreCompact exit 2 or `decision:block` stops the compaction. | R7: the payload order. D:887 (PreCompact can block). |
| The compact boundary is appended to the current file. It is parentless, `logicalParentUuid` points to the last record, and it has `content:"Conversation compacted"`, `level:"info"`, `isMeta:false`. | T: 46 of 65 boundaries appear part-way down a file. R7. |
| `compactMetadata {trigger, preTokens, durationMs, preservedSegment{headUuid, anchorUuid, tailUuid}, preservedMessages{anchorUuid, uuids, allUuids}, postTokens}`. `uuids` are the last N records before the boundary (N ranges from 2 to 21), and the anchor is the summary. | T: all 64 metadata objects that have `preservedMessages`, with the anchor being the summary in 100% of them. R7 (manual, N=2). |
| The summary (`isCompactSummary`) chains to the boundary. SessionStart:compact's attachments come after it, and the turn goes on. | T: 46 mid-file compactions. R7. |
| A fork of a compacted session contains: the verbatim boundary, whatever comes before the summary, the summary, the preserved records re-chained after the summary, then the rest. | T: all 19 transcripts that open on a `compact_boundary` |
| A fork of an uncompacted session copies every record with its parents unchanged. `sessionId` is rewritten to the fork's id. | R8: the copied records differ from the original only in `sessionId`. T: every 2.1.280+ shared origin carries the file's own id. This contradicts review correction #15, which older 2.1.219/229 copies (3 records) support. |
| `--resume <unknown>` prints `"No conversation found with session ID: <id>"` on stderr and, in stream-json mode, a `result/error_during_execution` frame. It exits 1. SessionEnd fires and SessionStart does not. | R8. B: the string. |

## Not modelled

These are documented as gaps, not faked:

- `prompt_id`, `permission_mode` and SessionStart `model`/`session_title` on
  payloads.
- A sub-agent's own background work outliving its final response.
- The resume-time notification for a task the previous process left running.
- StopFailure.
- Stream frames other than `task_notification`.
- The real `rendered` fields on `queued_command`.
- Separate stdout and stderr for Bash (the mock runs one combined stream).
- Token counts, which are 0.
- The foreground Agent tool_result body: the mock keeps `agentId: <id>\nagentType: <type>\n<result>`, while the real one is a `[Subagent hand-back]` frame with `<usage>`.
