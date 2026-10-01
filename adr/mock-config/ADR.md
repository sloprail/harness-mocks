---
concern: where a mock reads its configuration (flags and environment variables)
sloprails: [file-guard/config-at-entry, file-guard/adr-conformance, file-guard/no-such-rule]
# Files that still read or write the process environment themselves. The list
# only shrinks.
exceptions:
  - claude-mock/internal/hooks/invoker_exec.go
  - claude-mock/internal/hooks/plugin_cache.go
  - claude-mock/internal/runner/agent.go
  - claude-mock/internal/runner/background_bash.go
  - claude-mock/internal/runner/runner_print.go
  - claude-mock/internal/runner/session_paths.go
  - claude-mock/internal/runner/stream.go
  - claude-mock/internal/toolexec/toolexec_bash.go
---

# A mock reads its configuration once, at its entrypoint

## Concern

Where a mock reads configuration: its command-line flags and the environment
variables it honours (for Claude Code, the harness's own variables such as
`CLAUDE_CONFIG_DIR` and `CLAUDE_CODE_STOP_HOOK_BLOCK_CAP`, and the mock's
`A10N_MOCK_*`).

## Decision

- A mock reads every flag and environment variable once, in its entrypoint
  package (`<harness>-mock/*.go`), into one configuration value passed down.
- No other code calls `os.Getenv`, `os.LookupEnv`, `os.Setenv`,
  `os.Unsetenv` or `os.Environ`.
- The names of the variables are the harness adapter's; what they control is
  a field of the configuration.
