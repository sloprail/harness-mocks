---
concern: the environment of every child process a mock starts
sloprails: [file-guard/subprocess-env]
exceptions:
  - claude-mock/internal/runner/runner.go
---

# One place builds every child process's environment

## Concern

The environment of every child process a mock starts.

Today five files build it five different ways, and commit 0a093af fixed one of
them drifting.

## Decision

- A child process's environment is built only by `core/procenv`.
- We will migrate the remaining call sites later.
