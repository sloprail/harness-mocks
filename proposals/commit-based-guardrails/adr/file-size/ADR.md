---
status: proposed
sloprails: [gate/file-size, file-guard/file-size]
limits:
  go: 150         # non-test Go
  go_test: 400    # *_test.go
# Files over their limit when this ADR was written. Each may not grow, and the
# list may only shrink (a split is a Sloprail-Refactor: move-only commit).
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

File size. In the sibling sloprail repo, 101 of 159 Go files grew past the
skills' own ~150-line ceiling, and 12 were created already oversized, because
the rule was prose nobody checked at write time. Large files are where
near-duplicate logic hides and drifts.

## Decision

- A non-test Go file has at most `limits.go` lines. A test file has at most
  `limits.go_test` lines.
- A file listed under `exceptions` may not grow.
- It is checked before a Write or Edit lands (`gate/file-size`), and again on
  every commit (`file-guard/file-size`), which also sees shell writes and
  generated files.

## Consequences

New code is split by responsibility from the start. Legacy files shrink
through move-only refactors, and each split removes its line from
`exceptions`.
