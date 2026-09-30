---
concern: the size of Go files
sloprails: [gate/file-size, file-guard/file-size]
limits:
  go: 150         # non-test Go
  go_test: 400    # *_test.go
# Files over their limit. Each does not grow; the list only shrinks.
exceptions:
  - claude-mock/internal/runner/runner.go            # 179: Run alone is ~163
  - claude-mock/internal/runner/control_compact.go   # 164: compact alone is ~151
  - claude-mock/internal/runner/stream_turn.go       # 152: runOneTurnSig alone is ~140
---

# Go files stay small

## Concern

The size of every Go file in the repository.

## Decision

- A non-test Go file has at most `limits.go` lines. A test file has at most
  `limits.go_test` lines.
- A file listed under `exceptions` does not grow.
- New code is split by responsibility into files within the limit.
