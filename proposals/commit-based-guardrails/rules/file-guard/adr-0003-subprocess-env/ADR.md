---
status: proposed
# Sites that set a child process's environment themselves, as of this ADR.
# Each may not add sites, and the list may only shrink.
exceptions:
  - claude-mock/internal/hooks/invoker.go
  - claude-mock/internal/runner/background.go
  - claude-mock/internal/runner/runner.go
  - claude-mock/internal/runner/stream.go
  - claude-mock/internal/toolexec/toolexec.go
---

# ADR-0003: one place builds every child process's environment

**Concern.** The environment of every child process a mock starts: hook
commands, the Bash tool, the scenario script, background tasks. A real harness
exports the same session facts to all of them (for Claude Code,
`CLAUDE_CODE_SESSION_ID`, `CLAUDECODE`, `CLAUDE_CODE_ENTRYPOINT`, …).

Today six places in five different ways set it: `buildEnv` in `runner.go` and
`stream.go`, inline appends in `background.go` and `hooks/invoker.go`, and
`bashEnv` in `toolexec.go`. Commit `0a093af` fixed one of them having drifted:
the Bash tool ran without `CLAUDE_CODE_SESSION_ID`, while hooks and the
scenario script had it.

**Decision.**
- A child process's environment is built only by `core/procenv`: one
  function, taking the kind of process and the session.
- The facts a given harness exports come from that harness's adapter, which
  hands them to `core/procenv`.
- Nothing else assigns `cmd.Env`.

**Consequences.** A new kind of child process gets the harness's full
environment by construction. The five legacy sites move to `core/procenv` and
leave `exceptions`.
