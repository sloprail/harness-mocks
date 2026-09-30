---
status: proposed
sloprails: [file-guard/subprocess-env]
# Files that set a child process's environment themselves, as of this ADR.
# None may add assignments, and the list may only shrink.
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
tool, the scenario script, background tasks. A real harness exports the same
session facts to all of them (for Claude Code: `CLAUDE_CODE_SESSION_ID`,
`CLAUDECODE`, `CLAUDE_CODE_ENTRYPOINT`, …).

Today five files build it five different ways: `buildEnv` (`runner.go`,
`stream.go`), inline appends (`background.go`, `hooks/invoker.go`) and
`bashEnv` (`toolexec.go`). Commit `0a093af` fixed one of them drifting: the
Bash tool ran without `CLAUDE_CODE_SESSION_ID`, while hooks and the scenario
script had it.

## Decision

- A child process's environment is built only by `core/procenv`, from the
  kind of process and the session.
- The facts a harness exports come from that harness's adapter, which hands
  them to `core/procenv`.
- No other code assigns `cmd.Env`.

## Consequences

A new kind of child process gets the harness's full environment by
construction. The legacy files move to `core/procenv` and leave
`exceptions`.
