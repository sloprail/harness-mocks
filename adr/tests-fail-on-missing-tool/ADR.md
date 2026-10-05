---
concern: what a test does when a tool it needs is not installed
sloprails: [file-guard/adr-conformance, file-guard/tests-fail-on-missing-tool]
---

# A test whose required tool is missing fails

## Concern

A test that needs an external tool (an interpreter, a command-line program, a
harness binary) and finds it missing.

## Decision

- A test whose required tool is missing fails; it never skips.
