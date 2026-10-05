---
concern: which captured samples of a recorded run a mock's replay command checks
sloprails: [file-guard/adr-conformance]
---

# A replay replays every captured sample of a run

## Concern

A recorded run (`<harness>-mock/snapshots/runs/<name>/`) can hold several
captured samples (`samples/<ts>/`), each one real capture of the harness. If a
mock's replay command checks only some of them, "replays green" promises
different things from one mock to the next, and a sample that differs from the
mock is never noticed.

## Decision

- A mock's replay command replays every captured sample of a recorded run, and
  "replays green" means all of them do; a mock that replays only the newest
  sample is a gap to close, not a convention.
- The cursor mock's replay (`cursor-mock/internal/replay`) replays every sample.
  The claude and codex mocks' replays, which replay only the newest sample, are
  to be brought to the same.
