---
concern: what a mock does with a flag, option, input or feature of the real harness that it does not implement
sloprails: [file-guard/adr-conformance]
---

# A mock refuses what it does not implement

## Concern

What `claude-mock`, `codex-mock` and `cursor-mock` do with an input of the real
harness (a command-line flag, a configuration key, a feature switch) that they
do not implement. Ignoring it silently lets a wrong recording or test pass: a
run made with `--enable multi_agent_v2` replayed on the default mode, an output
schema that shapes nothing.

## Decision

- `claude-mock`, `codex-mock` and `cursor-mock` fail fast: every flag, option,
  input or feature of the real harness that the mock does not implement is
  refused loudly, with an error that names it, never accepted and ignored.
