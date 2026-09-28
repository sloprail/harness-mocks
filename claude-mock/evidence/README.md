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
| `stream.jsonl` | The stdout stream. `system/init`, `hook_started`/`hook_response`/`status` and `rate_limit_event` frames are dropped. |
| `transcript/` | The session's transcript files and `.meta.json` sidecars. Bookkeeping records (`queue-operation`, `last-prompt`, `mode`, …) and context attachments (`environment`, `skill_listing`, `deferred_tools_*`, `date`, …) are dropped. Every other record is kept verbatim. |
| `stderr.txt`, `exit.txt` | Where the run has them. |

## Sanitising

- The run directory is written `<RUN>`, and its encoded form `-RUN-`.
- The temp root is written `<TMP>/claude-<uid>`.
- The home directory is written `<HOME>`, and the user name `<user>`.

`agent-output-files.txt` is different: it lists the real (not fake) `HOME`'s
task directories and shows that `tasks/<agentId>.output` is a symlink to the
sub-agent's JSONL. The fake-HOME runs left 0-byte regular files there
instead, so that one behaviour could not be reproduced hermetically.
