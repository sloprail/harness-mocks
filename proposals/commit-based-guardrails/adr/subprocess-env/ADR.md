---
concern: the environment of every child process a mock starts
sloprails: [file-guard/subprocess-env]
# Files that still assign cmd.Env. None adds an assignment; the list only shrinks.
exceptions:
  - claude-mock/internal/hooks/invoker.go
  - claude-mock/internal/runner/background.go
  - claude-mock/internal/runner/runner.go
  - claude-mock/internal/runner/stream.go
  - claude-mock/internal/toolexec/toolexec.go
---

# One place builds every child process's environment

## Concern

The environment of every child process a mock starts: hook commands, the Bash
tool, the scenario script and background tasks. A harness exports the same
session facts to all of them (for Claude Code: `CLAUDE_CODE_SESSION_ID`,
`CLAUDECODE`, `CLAUDE_CODE_ENTRYPOINT`).

## Decision

- A child process's environment is built only by `internal/procenv`, from the
  kind of process and the session.
- The facts a harness exports come from that harness's adapter, which passes
  them to `internal/procenv`.
- No other code assigns `cmd.Env`, and no child process gets `os.Environ()`
  directly.
