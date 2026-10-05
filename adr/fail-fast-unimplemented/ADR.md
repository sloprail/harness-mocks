---
concern: what a mock does with a flag, option, input or feature of the real harness that it does not implement
sloprails: [file-guard/adr-conformance]
# Files that still accept a flag or input they implement nothing of. None adds
# one; the list only shrinks (the claude and cursor mocks follow this ADR next).
exceptions:
  - claude-mock/run_flags.go
---

# A mock refuses what it does not implement

## Concern

A mock stands in for a real harness in tests and recordings. Where it takes an
input of the real harness (a command-line flag, a configuration key, a feature
switch, a scenario field) and implements nothing of it, the silent alternative
lets a wrong recording or test pass: a run made with `--enable multi_agent_v2`
replayed on the default mode, an output schema that shapes nothing.

## Decision

- A mock fails fast: any flag, option, configuration key, input or feature it
  does not implement is refused loudly, with an error that names it, before
  anything runs. It is never accepted and ignored.
- A flag or option is implemented when it changes what the mock does as the
  real harness's does (or the mock models the part it changes); a flag that
  only exists so a command line works unchanged is not implemented, and is
  refused.
- What the mock leaves out is declared as a capability deviation
  (adr/modeled-surface), and the deviation says the mock refuses it.
- The refusal is covered by a test that gives the mock the input and sees the
  error and that nothing ran.
