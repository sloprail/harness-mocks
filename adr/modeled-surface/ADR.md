---
concern: which parts of a real harness a mock models, and how a capability states what it leaves out
sloprails: [file-guard/capability-rigor, file-guard/adr-conformance]
---

# A mock models the non-interactive harness its tests drive

## Concern

A real harness has more surface than its mock: interactive commands, tools
for other platforms, tools no test of ours drives. A capability's docs
describe all of it. Where the mock leaves part of it out, the gap must be
visible and deliberate, not a test nobody wrote.

## Decision

- A mock models the harness as its tests drive it: a non-interactive session
  (print mode, or a stream of prompts), with the tools those sessions use.
- Out of the model, for every harness: interactive commands (Claude Code's
  `/clear`, for example), tools for a platform the mocks do not run on
  (Claude Code's PowerShell tool), and tools no test drives yet (Claude Code's
  Monitor tool).
- A capability whose docs describe behaviour on a part left out declares it in
  that harness's cell, as a `deviations` entry citing this ADR and naming the
  part. Its tests prove the rest.
- Modelling a part that is out takes it out of this list and out of every
  `deviations` entry citing it, in the same change.
