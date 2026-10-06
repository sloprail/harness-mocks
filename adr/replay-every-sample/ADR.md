---
concern: which captured samples of a recorded run a mock's replay test checks, and what "replays green" means for a run
sloprails: [file-guard/replay-exceptions-only-shrink]
---

# A replay replays every captured sample of a run

## Concern

A recorded run (`<harness>-mock/snapshots/runs/<name>/`) can hold several
captured samples (`samples/<ts>/`), each one real capture of the harness. If a
replay checks only some of them, "replays green" promises different things from
one mock to the next, and a sample that differs from the mock is never noticed.
The replay test of each mock (`generated_replay_test.go`) is what decides it, so
that test is held to the copy the `replay-exceptions-only-shrink` file-guard
pins.

## Decision

- A mock's replay command replays every captured sample of a recorded run, and
  "replays green" means all of them do.
- A mock that replays only the newest sample is a gap to close, not a convention.

## Source

The user's words: "A mock's replay command replays every captured sample of a recorded run, and "replays green" means all of them do; a mock that replays only the newest sample is a gap to close, not a convention."
