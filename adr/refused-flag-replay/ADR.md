---
concern: how a recorded run made with a flag the mock refuses is replayed
sloprails: [file-guard/adr-conformance]
---

# A refused-flag recording replays as a check that the mock refuses the flag

## Concern

A recorded run can be made with a flag its mock refuses (adr/fail-fast-unimplemented):
the run has no mock behaviour to compare, so it would otherwise sit on the list of
recordings that do not replay green.

## Decision

- A recorded run made with a flag the mock refuses replays as a check that the mock
  refuses that flag; it is not an exception-list entry.

## Scope

It governs the replay of a recorded run of any mock: the claude mock's adapter
(`claude-mock/internal/replay`) marks such a run by the flag in its `setup/args`
(`refused` in `args.go`), and the replay test (`claude-mock/e2e/018_replay`) runs the mock
with that flag and checks that it exits non-zero and names the flag in its error, the script
never running; the run has no entry in the list of recordings that do not replay green.

Every mock's replay test follows it: a refused-flag recording is never an entry of its list of recordings that do not replay green.
