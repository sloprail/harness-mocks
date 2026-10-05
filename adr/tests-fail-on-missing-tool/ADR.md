---
concern: what a test does when a tool it needs is not installed
sloprails: [file-guard/adr-conformance, file-guard/tests-fail-on-missing-tool]
---

# A test whose required tool is missing fails

## Concern

A test that needs an external tool (an interpreter, a command-line program, a
harness binary) and finds it missing.

## Decision

- A Go test (`*_test.go`) whose required tool is missing fails; it never
  skips. A missing tool is one that `exec.LookPath` does not find; the test
  calls `t.Fatalf` saying what to install, and never `t.Skip`, `t.Skipf` or
  `t.SkipNow` after the lookup.
- A skip behind an explicit opt-in is not a skip on a missing tool: one inside
  an `if` that reads an environment variable (such as
  `A10N_REAL_CLAUDE_SUBAGENT_TEST`) or `testing.Short()` may skip.
