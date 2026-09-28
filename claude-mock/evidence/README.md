# Controlled-run fixtures

Each directory here is one controlled `claude -p` run of Claude Code 2.1.282.
EVIDENCE.md cites these runs as `F:<dir>`.

## How the runs were made

- **Hermetic.** Each run used a fake `HOME` with only `~/Library` linked, which
  keeps the login. So no user settings, global hooks, plugins, MCP servers or
  memory were loaded; only the project's own `.claude/settings.json` applied.
- **Command:** `claude -p --model haiku --dangerously-skip-permissions --output-format stream-json --verbose`.
- **Hooks:** every event runs `hook.sh`. It appends its stdin payload to
  `payloads.jsonl` and, in the runs that need it, prints output or exits
  non-zero.
- **Where:** a scratch git repository with one commit.

## Files in each directory

| File | Contents |
|---|---|
| `payloads.jsonl` | Every hook payload, in firing order. |
| `stream.jsonl` | The stdout stream. Dropped: the account-specific `system/init` and `system/commands_changed` frames, the `hook_started`/`hook_response`/`status` frames and `rate_limit_event`. |
| `transcript/` | The session's transcript files and `.meta.json` sidecars. **Every record is kept**, so every uuid chain and every "never written" claim can be checked. Context attachments that are machine- or account-specific (`environment`, `session_context`, `credential_org`, `skill_listing`, `deferred_tools_*`, `agent_listing_delta`, `mcp_instructions_delta`, `prompt_snapshot`, `remote_session_change`, …) keep their record, `type` and uuid chain, but their payload and `rendered` text are replaced with a `redacted` note. Thinking blocks' `signature` blobs are replaced with `<redacted>`. |
| `settings.json` | The run's project `.claude/settings.json`: which events had a hook. |
| `stderr.txt`, `exit.txt` | Where the run has them. |

## Sanitising

- The run directory is written `<RUN>`, and its encoded form `-RUN-`.
- The temp root is written `<TMP>/claude-<uid>`.
- The home directory is written `<HOME>`, and the user name `<user>`.

`agent-output-files.txt` is different: it lists the real (not fake) `HOME`'s
task directories and shows that `tasks/<agentId>.output` is a symlink to the
sub-agent's JSONL. The fake-HOME runs left 0-byte regular files there
instead, so that one behaviour could not be reproduced hermetically.
