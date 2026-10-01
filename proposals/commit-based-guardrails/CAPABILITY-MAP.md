# Capability map: what `claude-mock` models today

Source of truth for the 47 files in `spec/capabilities/`. Every doc anchor in them was checked
against the fetched page (heading → lowercase, spaces → `-`, other characters dropped); every
`runs:` entry names an existing `claude-mock/evidence/<fixture>/`. Evidence letters are those of
`claude-mock/EVIDENCE.md` (F fixture, T transcripts, B binary, D hooks doc).

**Path shorthands.** Code paths are under `claude-mock/`. `runner/` = `internal/runner/`, `hooks/` =
`internal/hooks/`, `toolexec/` = `internal/toolexec/`. Tests are under `claude-mock/`, e2e ones as
`e2e/<dir>/<file>`:

| tag | file |
|---|---|
| t001 | `e2e/001_passthrough/test_001_01_passthrough_test.go` |
| t002w | `e2e/002_worktree/test_002_worktree_test.go` |
| t002h | `e2e/002_hooks/test_002_01_worktree_hooks_test.go` |
| t003s | `e2e/003_session/test_003_session_test.go` |
| t003p | `e2e/003_session/test_003_02_transcript_path_symlink_resolved_test.go` |
| t003x | `e2e/003_toolexec/test_003_01_toolexec_test.go` |
| t004 · t005 · t006 · t007 · t008 | `e2e/004_subagent/test_004_subagent_test.go` · `005_pretooluse/…` · `006_posttooluse/…` · `007_toolexec/…` · `008_session_file/…` (each `test_NNN_*_test.go`) |
| t009a · t009b · t009c · t009d · t009e | `e2e/009_agent_tool/test_009_01_agent_tool_test.go` · `_03_subagent_worktree_cwd` · `_05_worktree_binds_real_directory` · `_06_hook_claude_env` · `_07_subagent_bash_session_id` (`_04_…_realclaude` needs a real claude) |
| t010 · t011 · t012 · t013 · t014 · t015 · t016 | `e2e/010_subagent_block_loop/…` · `011_plugin_hooks/…` · `012_schedule_wakeup/…` · `013_compaction/…` · `014_sragent_flags/…` · `015_resume_prompt/…` · `016_transcript_chain/…` |
| t017a | `e2e/017_session_lifecycle/test_017_session_lifecycle_test.go` |
| t017b | `e2e/017_session_lifecycle/test_017_02_background_and_hooks_test.go` |
| u-inv · u-plug · u-set | `internal/hooks/{invoker,plugin,settings}_test.go` |
| u-tr · u-bg · u-fork · u-sess · u-tool | `internal/runner/{transcript,background,fork,session}_test.go` · `internal/toolexec/toolexec_test.go` |

Tests are cited by number (`t017b:26` = `TestT017_26_…`); a unit test by its name.

## 1. Capabilities

Docs are `page#anchor` under `https://code.claude.com/docs/en/`. Fixtures are `evidence/<name>`
(→ `snapshots/runs/<name>`). `—` = none.

| id | summary | implementing code | proving tests | fixtures | docs |
|---|---|---|---|---|---|
| hook-common-payload | every hook payload names session, transcript, cwd; sub-agent events name the agent | `hooks/invoker.go:Fire` (118), `SetAgent` (58), `SetTranscriptPath` (49); `hooks/event.go:Input` (44) | t017a:03, t017a:05, t004:07, t003s:04 | bgagent, meta | hooks#common-input-fields |
| session-start-hook | SessionStart fires with `source`; cannot block | `runner/runner.go:Run` (224-237), `fireSessionStart` (407) | t003s:01,02,03; t017a:01,02 | forkresume, compact, hookerrors | hooks#sessionstart-input, #sessionstart-decision-control, #exit-code-2-behavior-per-event |
| session-end-hook | SessionEnd fires, reason `other`, output unrecorded | `runner/runner.go:fireSessionEnd` (318) | t003s:06,07; t017b:17,19 | hookerrors, resume-unknown | hooks#sessionend, #sessionend-input |
| user-prompt-submit-hook | fires before the prompt; adds context or refuses; also for task notifications | `runner/runner.go:Run` (286-297), `runPrintMode` (339-350); `runner/background.go:submitNotification` (550) | t017b:19, t017b:28 | hookerrors, midturn, bgagent | hooks#userpromptsubmit-input, #userpromptsubmit-decision-control, #what-a-blocked-prompt-leaves-behind |
| stop-hook-payload | Stop fires at each end of turn with `stop_hook_active`, last message, background tasks | `runner/stream.go:streamAndHook` (88-100); `runner/runner.go:runPrintMode` (373-381) | t003s:04,05; t002h:03; t017b:11,12 | bgbash, bgagent, stops | hooks#stop, #stop-input |
| stop-block-continuation | a Stop block re-prompts the turn; one result frame | `runner/stream.go:streamAndHook` (103-113), `withEmptyResult` (158); `runner/transcript.go:stopHookFeedback` (491), `writeStopSummary` (455) | t002h:06,07; t017b:14 | stops, cap | hooks#stop-decision-control, #exit-code-2-behavior-per-event |
| stop-block-cap | consecutive Stop/SubagentStop blocks capped; next is overridden | `runner/agent.go:stopHookBlockCap` (51), `subagentRun.execute` (269-288); `runner/stream.go:streamAndHook` (105-122), `writeCapOverride` (172) | t017b:26,27; t010:04,05,06 | cap, cap-sub | hooks#stop-input, #stop-decision-control, #subagentstop-input; hooks-guide#stop-hook-hits-the-block-cap; env-vars#variables |
| subagent-stop-block-loop | a SubagentStop block re-runs the sub-agent with feedback | `runner/agent.go:subagentRun.execute` (251), `fireSubagentStop` (422), `subagentRun.invoker` (363) | t010:01-08 | cap-sub, hookmix | hooks#subagentstop, #subagentstop-input, #decision-control |
| subagent-lifecycle-hooks | SubagentStart (cannot block) / SubagentStop fire around a sub-agent | `runner/agent.go:execute` (258-267), `fireSubagentStop`; `runner/control.go:handleControlRecord` `subagent_start` (45) | t004:01-07; t002h:04,05; t017a:05; t017b:13 | meta, hookerrors, hookmix, bgagent | hooks#subagentstart, #subagentstart-input, #subagentstop, #subagentstop-input |
| hook-exit-code-semantics | exit 0 accepts, 2 blocks (per event), other = non-blocking error | `hooks/invoker.go:invokeCommand` (303-317), `BlockError`/`QuoteBlock` (173-199); `runner/transcript.go:recordHookRuns` (277) | u-inv `TestInvokeCommand_Exit2Blocks`; u-tr `TestRecordHookRuns_Exit2PerEvent`, `_NonBlockingError`, `_EmptyStderrOnExit2`; t003s:03; t004:04; t006:03; t017b:16 | hookerrors, hookmix, stops | hooks#exit-code-output, #exit-code-2-behavior-per-event, #other-exit-codes |
| pretooluse-refusal | PreToolUse deny/exit 2 → error tool_result, tool not run, turn goes on | `runner/stream.go:scanLines` (456-486), `runOneTurnSig` (239-247); `runner/transcript.go:isDeny` (422), `denyReason` (430) | t005:02,05; u-tr `TestRecordHookRuns_PreToolUseDenyLeavesNothing` | stops, bashfail | hooks#pretooluse-decision-control, #exit-code-2-behavior-per-event |
| posttooluse-payload | PostToolUse with `tool_response`, `duration_ms` | `runner/stream.go:runOneTurnSig` (314-325), `toolResponse` (375), `scanLines` (514-528) | t006:01,02,03; t017b:20,21 | bgbash, bgagent | hooks#posttooluse, #posttooluse-input |
| tool-failure-hook | PostToolUseFailure instead of PostToolUse when a tool ran and failed | `runner/stream.go:runOneTurnSig` (301-313); `toolexec/toolexec.go:executeBash` (108, `Failed`) | t017b:29 | bashfail, stops | hooks#posttoolusefailure, #posttoolusefailure-input |
| hook-additional-context | hook JSON `additionalContext` reaches the agent and is recorded | `runner/runner.go:additionalContextFrom` (424), `fireSessionStart` (414-417), `emitSystemContext` (434); `runner/transcript.go:additionalContext` (410) | t017b:15; u-tr `TestRecordHookRuns_AdditionalContextPair` | hookmix, stops | hooks#add-context-for-claude |
| hook-output-transcript-records | hook output/errors/blocks leave transcript records; silent ones none; Stop summary | `runner/transcript.go:recordHookRuns` (277), `recordedEvents`, `writeStopSummary` (455) | all `TestRecordHookRuns_*` in u-tr; t017a:04; t017b:14,16,19 | hookerrors, hookmix, stops, bgbash | none (T/B evidence only) |
| hook-matcher-filter | hook runs only when its matcher fits the event subject | `hooks/settings.go:EntriesFor` (84), `matchesEntry` (94) | t005:03 | — | hooks#matcher-patterns |
| hooks-all-matching-run | all matching hooks run; first block wins | `hooks/invoker.go:Fire` (154-164), `mergeOutput` (349) | u-inv `TestFire_RunsEveryHandlerAndReturnsTheFirstBlock` | — | hooks#hook-handler-fields; hooks-guide#combine-results-from-multiple-hooks |
| hook-command-handler | command hook runs via shell in event cwd, payload on stdin | `hooks/invoker.go:invokeCommand` (221-289) | u-inv `TestInvokeCommand_QuotedPathRunsViaShell`, `_ArgsAndEnvRunViaShell`; t005:04 | stops, hookerrors (payloads.jsonl) | hooks#command-hook-fields, #hook-input-and-output |
| hook-timeout | hook bounded by timeout; kills its process tree | `hooks/invoker.go:invoke` (201), `invokeCommand` (237-270; `defaultHookTimeout` line 19) | u-inv `TestInvokeCommand_TimeoutBoundsTheCall`, `_TimeoutKillsGrandchild`, `_FastHookNotKilled`, `_HookRunsInItsOwnProcessGroup` | — | hooks#timeouts, #command-hook-fields |
| http-hooks | HTTP handler gets payload as POST; failures non-blocking | `hooks/invoker.go:invokeHTTP` (320) | none | — | hooks#http-hook-fields, #http-response-handling |
| plugin-hooks | enabled plugins' hooks resolved via declared marketplace | `hooks/plugin.go:loadPluginHooks` (99), `resolveMarketplace` (196), `expandPluginRoot` (333); `hooks/settings.go:LoadSettings` (41) | t011:01,02,03; u-plug (all); u-set (all) | — | hooks#hook-locations; plugin-marketplaces#add-plugin-entries |
| worktree-hooks | hooks at worktree create/remove; failing create aborts | `runner/control.go:handleControlRecord` (29-43) | t002w:01-07; t002h:01,02 | — | hooks#worktreecreate, #worktreeremove |
| subprocess-session-env | tool/hook subprocesses see harness + session env | `hooks/invoker.go:invokeCommand` (282-285); `toolexec/toolexec.go:bashEnv` (122); `runner/background.go:launchBash` (211); `runner/stream.go:buildEnv` (550) | t009d; t009e; t007:11; u-tool (both); u-inv `TestInvokeCommand_SetsClaudeCodeEnvOnHook`, `_SetsHarnessEnvWithoutSessionID` | — | env-vars#variables |
| subagent-transcripts | sub-agent records in own file + `.meta.json` sidecar | `runner/session.go:seedSubagentTranscript` (404), `subagentMeta` (378); `runner/agent.go:prepareSubagent` (181-203); `runner/transcript.go:openTranscript` (103) | t017a:05; t017b:30; t014:06; u-sess `TestSeedSubagentTranscript*` | meta, hookerrors, bgagent | claude-directory#application-data; sub-agents#resume-subagents |
| foreground-subagent-result | foreground sub-agent's report returned as the tool result | `runner/agent.go:runAgentTool` (112), `buildAgentResult` (545), `sectionHash` (583), `lastResultText` (504) | t009a:01,02 (Task alias); t017b:25; u-fork `TestBuildAgentResult`, `TestSectionHashMatchesTheBinary` | meta, hookerrors, hookmix | sub-agents#run-subagents-in-foreground-or-background; tools-reference#agent-tool-behavior |
| nested-subagents | sub-agents dispatch sub-agents; depth/parent recorded | `runner/agent.go:prepareSubagent` (194-198), `subagentRun.run` (320-347) | t009a:03; t017b:13 | meta | sub-agents#let-subagents-spawn-their-own-subagents |
| subagent-worktree-isolation | sub-agent runs in its own worktree/branch, shared session id | `runner/agent.go:prepareSubagent` (166-179), `bindWorktree` (382) | t009b (both); t009c; t017b:30 | meta | sub-agents#write-subagent-files; worktrees#isolate-subagents-with-worktrees |
| agent-input-validation | missing `description`/`prompt` → InputValidationError | `runner/agent.go:prepareSubagent` (134-147); `runner/background.go:inputValidationError` (634) | t017b:12c; u-bg `TestInputValidationError` | — (B/T only) | none |
| background-bash | `run_in_background` Bash: receipt, output file, concurrent | `runner/background.go:launchBash` (179), `tasksDir` (142), `changesDirectory` (159), `running` (331) | t017b:11,11b; u-bg `TestLaunchBash_*`, `TestChangesDirectory`, `TestTasksDir` | bgbash, midturn | tools-reference#background-commands; interactive-mode#how-backgrounding-works |
| background-agent | `run_in_background` Agent: receipt, concurrent sub-agent | `runner/background.go:launchAgent` (277); `runner/agent.go:execute` (251) | t017b:12,12b,12d | bgagent | sub-agents#run-subagents-in-foreground-or-background |
| task-notifications | finished task handed mid-turn or as a new turn | `runner/background.go:notification` (420), `deliverMidTurn` (512), `deliverAsTurn` (529), `awaitAfterTurn` (388); `runner/stream.go:streamAndHook` (129-140) | t017b:11,12,28; u-bg `TestNotification_Bash`, `_Agent` | midturn, bgagent | sub-agents#run-subagents-in-foreground-or-background; interactive-mode#how-backgrounding-works; hooks#userpromptsubmit-decision-control |
| background-bash-reaped-at-exit | `-p`: leftover background Bash is killed | `runner/background.go:stopOwned` (565), `shutdown` (588), `killGroup` (609); `runner/stream.go:streamAndHook` (141) | t017b:11c; u-bg `TestShutdown_KillsTheProcessGroup` | bgbash | headless#background-tasks-at-exit |
| print-waits-for-background-agents | `-p` waits for background agents, one turn each | `runner/background.go:awaitAfterTurn` (388), `agentsRunning` (370); `runner/stream.go:streamAndHook` (129-143) | t017b:12; u-bg `TestAwaitAfterTurn_WaitsForAgentsInLaunchOrder` | bgagent | headless#background-tasks-at-exit |
| foreground-subagent-bash-ends-with-response | a foreground sub-agent's background command dies at its final response | `runner/background.go:launchBash` (248-263), `stopOwned` (565); `runner/stream.go:streamAndHook` (85) | u-bg `TestLaunchBash_InsideAForegroundSubagent` | — (B only) | tools-reference#background-commands |
| task-stream-frames | `task_started`/`task_updated`/`task_notification` stream frames | `runner/background.go:writeFrame` (493), `writeTaskEndFrames` (475), `launchBash` (221); `runner/agent.go:execute` (253, 297, 301); `runner/stream.go:ownedBashFrames` (344) | t017b:11,12,12d,31; u-bg `TestStreamLinesStayWholeUnderConcurrentFrames` | bgbash, midturn, bgagent, meta, hookerrors, cap-sub | agent-sdk/typescript#sdktaskstartedmessage, #sdktaskupdatedmessage, #sdktasknotificationmessage |
| manual-compaction | compaction with Pre/Post hooks, PreCompact can stop it | `runner/control.go:compact` (98), `compactHookLines` (223); `runner/stream.go:scanLines` (430) | t017a:07,07b; t013:01,02,03 | compact, compact-nohooks | hooks#precompact, #postcompact |
| compaction-transcript-continuity | boundary + summary + preserved tail keep the chain walkable | `runner/control.go:writeCompactBoundary` (266), `lastCumulativeDropped` (353); `runner/transcript.go:lastUUIDs` (222) | t017a:07,07c,07d,07e,10; u-tr `TestLastUUIDs` | compact, compact-nohooks | none (F/T evidence only) |
| session-resume | `--resume` continues the transcript; session hooks only | `run.go:rootRunE` (128-150); `runner/session.go:openRunTranscript` (438), `findSessionFile` (479), `appendResumePrompt` (537) | t017a:06; t015:01-04; t016:04; t003s:02; t004:01,05; u-sess `TestAppendResumePromptTranscript_*`; u-fork `TestFindSessionFile` | forkresume, compact | cli-reference#cli-flags; agent-sdk/sessions#resume-by-id; headless#continue-conversations |
| session-resume-unknown | resuming a missing session: message, exit 1, SessionEnd only | `run.go:noConversation` (220); `runner/runner.go:Run` (182-196), `ErrNoConversation` (144) | t017b:17; t001:05,06 (`A10N_MOCK_NO_RESUME`) | resume-unknown | cli-reference#cli-flags; agent-sdk/sessions#resume-by-id |
| session-fork | `--fork-session`: new id + transcript carrying history | `runner/session.go:forkTranscript` (591), `forkSegment` (625), `preservedUUIDs` (690); `run.go:rootRunE` (143-149) | t017a:08,09; t017b:22; u-fork `TestForkSegment_*`, `TestForkTranscript_*` | forkresume | cli-reference#cli-flags; agent-sdk/sessions#fork-to-explore-alternatives |
| session-transcript-file | transcript path from config dir + resolved cwd + id; lazy creation | `runner/session.go:sessionFilePath` (52), `resolveEncodingCwd` (63), `resolveConfigDir` (23); `runner/transcript.go:transcript` (41), `openTranscript` (103) | t008:01-05; t003p; t017a:01,02,04; t016:01,03 | stops, forkresume, hookerrors | claude-directory#application-data; env-vars#variables |
| transcript-record-envelope | uniform bookkeeping fields on every record | `runner/transcript.go:recordStamp` (62), `newRecordStamp` (84), `gitBranch` (90); `runner/sessionwriter.go:stampRecord` (167), `chainRecord` (97); `runner/session.go:writeRootPrompt` (515) | t017b:23; t016:01-04; u-tr `TestStampRecord_MainAndSidechain` | bgbash, midturn, bgagent | none (T/F evidence only) |
| empty-tool-result-placeholder | empty result → `(<Tool> completed with no output)` | `runner/stream.go:emitToolResult` (593) | t017b:24 | bashfail | none (T/B evidence only) |
| bash-tool-result | Bash output, structured result, `Exit code N` error | `toolexec/toolexec.go:executeBash` (79) | t007:01,02,03; t003x; t017b:29 | bashfail, bgbash | tools-reference#bash-tool-behavior |
| file-tools | Read/Write/Edit/Glob in the working directory | `toolexec/toolexec.go:executeRead` (139), `executeWrite` (173), `executeEdit` (197), `executeGlob` (231), `resolvePath` (252) | t007:04-10; t003x:03 | — | tools-reference#read-tool-behavior, #write-tool-behavior, #edit-tool-behavior, #glob-tool-behavior |
| schedule-wakeup | ScheduleWakeup validated and acknowledged | `runner/schedulewakeup.go:runScheduleWakeupTool` (43); `runner/stream.go:runOneTurnSig` (270) | t012:01,02,03 | — | scheduled-tasks#let-claude-choose-the-interval |
| noninteractive-run | one prompt run to completion, JSONL events, one result | `run.go:rootRunE`; `runner/runner.go:Run` (151); `runner/stream.go:streamAndHook` (57); `runner/record.go:validateRecord` (95) | t001:01-04; t016:02 | stops, bashfail | cli-reference#cli-flags; headless#basic-usage, #stream-responses |

**Mock-only surface (no capability file; nothing real to cite):** the scenario protocol (a `--script`
re-run per turn, `A10N_MOCK_*` env, control records `compact` / `worktree_create` /
`worktree_remove` / `subagent_start`, the `maxIdenticalTurns` loop guard `runner/stream.go:43`),
the raw `--print` path (`runner/runner.go:runPrintMode` 335, `A10N_MOCK_PRINT_STREAM`), and the
accepted-but-ignored flags for sr-agent (`run.go:addRunFlags`, t014:01-03).

## 2. Proposed modules

The split follows what the code does. **Core** = a Cursor or Codex mock would reuse it unchanged.
**Adapter** = Claude wire format, flag/env names, record shapes, texts.

| module | responsibility | today's code | capabilities | side |
|---|---|---|---|---|
| `internal/hooks` | resolve which handlers match an event, run them (shell, HTTP, timeout, process-group kill), classify the outcome (accept / block / non-blocking error), merge results | `hooks/invoker.go:Fire` (118), `invoke` (201), `invokeCommand` (221), `invokeHTTP` (320), `mergeOutput` (349), `HandlerRun` (76), `BlockError` (173); `hooks/settings.go:EntriesFor` (84), `matchesEntry` (94) | hook-command-handler, hook-timeout, http-hooks, hooks-all-matching-run, hook-matcher-filter, hook-exit-code-semantics | shared-core |
| `adapters/claude/hookwire` | Claude's event names, payload/output JSON, exit-2 quoting, settings + plugin loading | `hooks/event.go` (all: `EventName`, `Input`, `Output`, `HookSpecificOutput`, `MarshalJSON` 170); `hooks/settings.go:LoadSettings` (41); `hooks/plugin.go` (all); `hooks/invoker.go:QuoteBlock` (194) | hook-common-payload, plugin-hooks, session-start-hook, session-end-hook, tool-failure-hook, posttooluse-payload, stop-hook-payload | claude-adapter |
| `internal/turnloop` | the turn: run the agent script, fire the end-of-turn hook, re-prompt on block with a cap, loop-guard, hand over finished tasks | `runner/stream.go:streamAndHook` (57), `runOneTurnSig` (198), `turnResult` (182), `withEmptyResult` (158); `runner/agent.go:stopHookBlockCap` (51) logic | stop-block-continuation, stop-block-cap, print-waits-for-background-agents | shared-core (cap env name is adapter) |
| `internal/toolcall` | intercept a tool call: fire pre-hook, refuse or execute, emit the result, fire post/failure hook | `runner/stream.go:scanLines` (409; PreToolUse 456-486), `runOneTurnSig` (231-335), `emitToolResult` (588), `toolResponse` (375), `pendingToolUse` (386) | pretooluse-refusal, posttooluse-payload, tool-failure-hook, empty-tool-result-placeholder | shared-core; result texts adapter |
| `internal/tools` | built-in tool execution | `toolexec/toolexec.go:Execute` (52), `executeBash` (79), `executeRead/Write/Edit/Glob` (139-231), `resolvePath` (252) | bash-tool-result, file-tools | shared-core; `Exit code N` text and `toolUseResult` shapes adapter |
| `internal/subagents` | dispatch a nested agent run: id, cwd or worktree, own transcript, start/stop hooks, block→re-run loop, result | `runner/agent.go:runAgentTool` (112), `prepareSubagent` (132), `subagentRun` (225), `execute` (251), `run` (320), `invoker` (363), `bindWorktree` (382), `fireSubagentStop` (422) | subagent-lifecycle-hooks, subagent-stop-block-loop, nested-subagents, subagent-worktree-isolation, foreground-subagent-result, agent-input-validation | shared-core |
| `adapters/claude/agenttool` | Claude's Agent/Task tool: input schema, `.meta.json`, hand-back text, trailer, section hash | `runner/agent.go:agentToolInput` (71), `buildAgentResult` (545), `sectionHash` (583), `newAgentID` (597), `isAgentTool` (61); `runner/session.go:subagentMeta` (378), `seedSubagentTranscript` (404) | subagent-transcripts, foreground-subagent-result | claude-adapter |
| `internal/tasks` | background-task registry: launch, own, list running, finish, deliver, kill at exit | `runner/background.go:backgroundTasks` (91), `add` (105), `finish` (111), `running` (331), `takeFinished` (353), `agentsRunning` (370), `awaitAfterTurn` (388), `deliverMidTurn` (512), `deliverAsTurn` (529), `stopOwned` (565), `shutdown` (588), `killGroup` (609) | background-bash, background-agent, task-notifications, background-bash-reaped-at-exit, foreground-subagent-bash-ends-with-response | shared-core |
| `adapters/claude/tasks` | receipts, notification XML, summaries, task stream frames, output-file layout | `runner/background.go:launchBash` texts (248-268), `launchAgent` text (298-303), `notification` (420), `status` (442), `summary` (456), `writeTaskEndFrames` (475), `writeFrame` (493), `tasksDir` (142), `changesDirectory` (159); `runner/stream.go:ownedBashFrames` (344) | task-stream-frames, background-bash, background-agent | claude-adapter |
| `internal/transcript` | append-only chained record file: lazy open, uuid chain, parent seeding, held records, last-N uuids | `runner/transcript.go:transcript` (41), `openTranscript` (103), `persist` (172), `lastUUIDs` (222), `holdHookRuns` (206); `runner/sessionwriter.go` (all) | session-transcript-file, transcript-record-envelope | shared-core |
| `adapters/claude/records` | Claude's record shapes: stamp constants, hook attachments, stop summary, feedback turn, preamble, root/resume prompt records | `runner/transcript.go:recordStamp` (62), `recordHookRuns` (277), `additionalContext` (410), `writeStopSummary` (455), `stopHookFeedback` (491), `recordedEvents`; `runner/session.go:mockPreambleRecords` (150), `writeRootPrompt` (515), `appendResumePrompt` (537); `runner/stream.go:writeCapOverride` (172) | hook-output-transcript-records, hook-additional-context, transcript-record-envelope | claude-adapter |
| `internal/session` | locate, resume, fork a session's transcript; no-such-session error | `runner/session.go:findSessionFile` (479), `sessionFilePathIfExists` (462), `openRunTranscript` (438), `forkTranscript` (591), `forkSegment` (625); `runner/runner.go:ErrNoConversation` (144) | session-resume, session-resume-unknown, session-fork | shared-core; project-dir encoding (`sessionFilePath` 52, `resolveEncodingCwd` 63) adapter |
| `internal/compaction` | the compaction sequence (pre-hook, summarizer stop, boundary, summary, start hook, post-hook), preserved-segment maths | `runner/control.go:compact` (98), `compactionSpec` (240), `writeCompactBoundary` (266), `lastCumulativeDropped` (353) | manual-compaction, compaction-transcript-continuity | shared-core sequence; `compactMetadata` shape adapter |
| `internal/procenv` (new) | one place that builds the environment of every child process the mock spawns | today scattered, see §3 | subprocess-session-env | shared-core; variable names adapter |
| `internal/scenario` | drive a scenario script: spawn per turn, validate records, route control records | `runner/stream.go:scanLines` (409), `runner/record.go` (all), `runner/control.go:handleControlRecord` (27), `runner/runner.go:Run` (151), `runPrintMode` (335), `lockedWriter` (447) | noninteractive-run, worktree-hooks, schedule-wakeup | shared-core; record validation adapter |
| `adapters/claude/cli` | `claude -p` command line and its error outputs | `main.go`, `run.go:addRunFlags`, `rootRunE`, `noConversation` | noninteractive-run, session-resume, session-resume-unknown, session-fork | claude-adapter |

## 3. Cross-cutting concerns implemented in several places

**A. Building a child process's environment.** All start from `os.Environ()`; they do not agree.

| site | child | adds |
|---|---|---|
| `runner/stream.go:559-571` (`buildEnv`; also used at `runner/runner.go:362`) | scenario script | `CLAUDE_CODE_SESSION_ID` (always, even if empty), `CLAUDE_CONFIG_DIR`, five `A10N_MOCK_*` |
| `toolexec/toolexec.go:123-126` (`bashEnv`) | foreground Bash | `CLAUDE_CODE_SESSION_ID` only when non-empty |
| `runner/background.go:211` | background Bash | `CLAUDE_CODE_SESSION_ID` always |
| `hooks/invoker.go:282-285` | hook command | `CLAUDECODE=1`, `CLAUDE_CODE_ENTRYPOINT=cli`, `CLAUDE_CODE_SESSION_ID` when non-empty |
| `e2etest/e2etest.go:85-87` | the mock itself | `CLAUDE_CODE_PLUGIN_CACHE_DIR`, `CLAUDE_CONFIG_DIR`, `CLAUDE_CODE_TMPDIR` |

Disagreements: (1) `CLAUDECODE=1` is set for hooks only, yet `env-vars#variables` says it is set in
Bash-tool subprocesses too; neither Bash site sets it. (2) session id is set when empty at two sites
and skipped at one. (3) hooks say `CLAUDE_CODE_ENTRYPOINT=cli` while every record is stamped `sdk-cli`
(`runner/transcript.go:79`); the hook env of a `-p` run is not measured. (4) `CLAUDE_CODE_CHILD_SESSION`
(documented) is set nowhere. (5) `CLAUDE_CODE_TMPDIR` is read at `runner/background.go:143` only.

**B. Spawning and killing a process tree.** `hooks/invoker.go:258-270` and `runner/background.go:214`,
`609-616` use `Setpgid` and kill the group; `toolexec/toolexec.go:85` (foreground Bash),
`runner/stream.go:199` and `runner/runner.go:360` (scripts) use plain `CommandContext`, which kills
only the shell. The two group-kill implementations are separate copies.

**C. Project-directory encoding and symlink resolution.** `runner/session.go:53` (transcript path) and
`runner/background.go:152` (task dir) both apply `nonAlphanumRe` to `resolveEncodingCwd(cwd)`. The cwd
is also resolved at `run.go:196`, and the tmp root at `runner/background.go:149`. They agree today, but
the encoding is written twice.

**D. Firing the same event from several places.** `hooks.Input{…}` is built at 18 sites with repeated
`SessionID`/`Cwd` boilerplate. Stop: `runner/stream.go:92` (real background tasks) vs
`runner/runner.go:373` (print mode, always empty `tasks`): they disagree. PostToolUse:
`runner/stream.go:315` (mock-run tools, with `duration_ms`) vs `:520` (scenario-written result, no
`duration_ms`, no `tool_input`). SubagentStop `runner/agent.go:426` uses `bg.running()` like Stop.

**E. The block cap.** One reader, `runner/agent.go:51`, and two loops: `runner/stream.go:105`
(`stopBlocks <= blockCap`) and `runner/agent.go:280` (`turn >= blockCap`). Both give cap+1 fires, so
they agree; only Stop writes the warning record (`runner/stream.go:172`), matching F:cap-sub.

**F. Prompt / resume-prompt records.** `runner/session.go:98` `seedRootPromptTranscript` vs `:515`
`writeRootPrompt`, and `:267` `appendResumePromptTranscript` vs `:537` `appendResumePrompt`: two
implementations of each. The first of each pair is called only from `session_test.go`, so the tests
`u-sess TestAppendResumePromptTranscript_*` prove the dead copy, not the live one.

**G. Id minting.** Agent ids `runner/agent.go:597` (`a` + 16 hex), task ids `runner/background.go:657`
(`b` + 8 alphanumerics, used at `:187` and `runner/stream.go:357`), uuids `runner/session.go:350`. Three
generators, no shared source; fine as shapes, but nothing injects determinism.

**H. Recording hook runs.** The recorder is swapped in three places: `runner/runner.go:212` (session
file), `runner/agent.go:366` (sub-agent file), `runner/control.go:120` (capture Pre/PostCompact runs so
they are not recorded). The rule "which events leave records" lives only in `recordedEvents`
(`runner/transcript.go`).

**I. Shelling out to git.** `runner/transcript.go:91,94` (branch), `runner/agent.go:386-396` (worktree),
`hooks/plugin.go:274` (clone): three call styles, no shared helper.

**J. Reading mock configuration from the environment.** `A10N_MOCK_SCRIPT` (`run.go:132`),
`A10N_MOCK_NO_RESUME` (`run.go:170`), `A10N_MOCK_SYSTEM_PROMPT` (`run.go:179`, written with `os.Setenv`),
`A10N_MOCK_PRINT_STREAM` (`runner/runner.go:355`), `A10N_MOCK_SUBAGENT_SCRIPT` (`runner/agent.go:411`),
`CLAUDE_CODE_STOP_HOOK_BLOCK_CAP` (`runner/agent.go:52`), `CLAUDE_CONFIG_DIR` (`runner/session.go:27`),
`CLAUDE_CODE_TMPDIR` (`runner/background.go:143`), `CLAUDE_CODE_PLUGIN_CACHE_DIR` (`hooks/plugin.go:48`).
Read at the point of use, not once into `Config`.

## 4. Gaps

**Modelled, but no evidence fixture** (`runs: []`): hook-matcher-filter, hooks-all-matching-run,
hook-timeout, http-hooks, plugin-hooks, worktree-hooks, subprocess-session-env,
agent-input-validation, foreground-subagent-bash-ends-with-response, file-tools, schedule-wakeup.
EVIDENCE.md has no row for hook-matcher-filter, hooks-all-matching-run (D:414 only), hook-timeout,
http-hooks, plugin-hooks, worktree-hooks, subprocess-session-env, file-tools or schedule-wakeup;
http-hooks also has no test. agent-input-validation and foreground-subagent-bash-ends-with-response
rest on B/T rows.

**Modelled, but no doc anchor** (`docs: []`, F/T/B evidence only): hook-output-transcript-records,
compaction-transcript-continuity, transcript-record-envelope, empty-tool-result-placeholder,
agent-input-validation. `capability-grounded` has nothing to check for these.

**Mock behaviour that departs from the real harness or has no evidence:**
- WorktreeCreate/WorktreeRemove are fired as notifications from a `worktree_create` control record
  (`runner/control.go:29`). Real WorktreeCreate replaces creation and its command hook prints the path
  (hooks#worktreecreate); nothing evidences the mock's shape. No record is left for either.
- A scenario-written `tool_result` triggers PostToolUse (`runner/stream.go:514`); no fixture.
- `system/hook_additional_context` stream frame for SessionStart (`runner/runner.go:434`); no fixture.
- `Output.Continue:false` is summarised in `stop_hook_summary` (`runner/transcript.go`) but stops nothing.

**"Not modelled" in EVIDENCE.md:** `prompt_id`, `permission_mode`, SessionStart `model`/`session_title`;
a sub-agent's background work outliving its response; the resume-time notification for a task left
running; StopFailure (constant declared, never fired); records left by PostToolUseFailure hooks;
summarizer SubagentStop of an automatic compaction and where its output is recorded; removing a clean
isolated worktree (sidecar becomes `worktreeCleanlyRemoved`); `tasks/<agentId>.output` for a foreground
sub-agent; the async receipt's `sharesCwd` line; stream frames `init`, `task_progress`,
`background_tasks_changed`, `notification`, `thinking_tokens`, `commands_changed`, hook frames; PostToolUseFailure
for non-Bash failures; foreground Bash frames of a foreground sub-agent; `rendered` on `queued_command`;
separate stdout/stderr for Bash; token counts; a foreground Agent's `usage`/`toolStats`.

**Hooks-doc events never fired by the mock** (from the fetched hooks page): Setup, InstructionsLoaded,
UserPromptExpansion, MessageDisplay, PermissionRequest, PostToolBatch, PermissionDenied, Notification,
TaskCreated, TaskCompleted, StopFailure, TeammateIdle, ConfigChange, CwdChanged, DirectoryAdded,
FileChanged, PreModelSwitch, PostModelSwitch, Elicitation, ElicitationResult.

**Hooks-doc capabilities not modelled:** PreToolUse `allow`/`ask`/`defer` and input rewriting
(the mock reads only deny); `permission_mode` on payloads; prompt, agent and MCP-tool hook types
(`hooks/invoker.go:invoke` ignores any type but `command` and `http`); async hooks; the `if` handler
field; matcher regexes and MCP-tool matchers (`hooks/settings.go:matchesEntry` is equality or `*`);
SessionStart `clear` source and env-file persistence (hooks#persist-environment-variables); `CLAUDE_CODE_SUBPROCESS_ENV_SCRUB`.
