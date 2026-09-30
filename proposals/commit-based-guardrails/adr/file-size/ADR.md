---
concern: the size of Go files
sloprails: [gate/file-size, file-guard/file-size]
limits:
  go: 150         # non-test Go
  go_test: 400    # *_test.go
# Files over their limit. Each does not grow; the list only shrinks.
exceptions:
  - claude-mock/internal/runner/session.go            # 701
  - claude-mock/internal/runner/background.go         # 667
  - claude-mock/internal/runner/stream.go             # 651
  - claude-mock/internal/runner/agent.go              # 603
  - claude-mock/internal/runner/transcript.go         # 520
  - claude-mock/internal/runner/runner.go             # 456
  - claude-mock/internal/runner/control.go            # 374
  - claude-mock/internal/hooks/invoker.go             # 368
  - claude-mock/internal/hooks/plugin.go              # 349
  - claude-mock/internal/toolexec/toolexec.go         # 257
  - claude-mock/run.go                                # 238
  - claude-mock/internal/runner/record.go             # 216
  - claude-mock/internal/runner/sessionwriter.go      # 203
  - claude-mock/internal/hooks/event.go               # 177
  - claude-mock/e2e/017_session_lifecycle/test_017_02_background_and_hooks_test.go  # 1137
  - claude-mock/e2e/017_session_lifecycle/test_017_session_lifecycle_test.go        # 644
  - claude-mock/e2e/010_subagent_block_loop/test_010_01_subagent_block_loop_test.go # 504
---

# Go files stay small

## Concern

The size of every Go file in the repository.

## Decision

- A non-test Go file has at most `limits.go` lines. A test file has at most
  `limits.go_test` lines.
- A file listed under `exceptions` does not grow.
- New code is split by responsibility into files within the limit.
