---
concern: what a Go test does when an external tool it needs is not installed
sloprails: [file-guard/tests-fail-on-missing-tool]
---

# A Go test whose required external tool is missing fails

## Concern

A Go test that needs an external tool (an interpreter, a command-line program,
a harness binary) and finds it missing.

## Decision

- A Go test whose required external tool (looked up with `exec.LookPath`) is
  missing fails; it never calls `t.Skip` for that.
- In a test package that looks up an external tool, no test may skip unless the
  skip sits behind an `A10N_*_TEST` gate.
