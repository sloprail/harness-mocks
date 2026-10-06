---
concern: which captured samples of a recorded run a mock's replay test checks
sloprails: [file-guard/adr-conformance]
---

# A replay replays every captured sample of a run

## Concern

A recorded run (`<harness>-mock/snapshots/runs/<name>/`) can hold several
captured samples (`samples/<ts>/`), each one real capture of the harness. If a
replay checks only some of them, "replays green" promises different things from
one mock to the next, and a sample that differs from the mock is never noticed.

## Decision

- Each mock's replay test replays every captured sample of a recorded run; a run
  counts as replaying green only when every sample does.
