---
concern: what grounds a capability: the real harness for what it does, the user for what is mocked
sloprails: [file-guard/capability-grounded]
---

# A capability states only what its harnesses document or were recorded doing

## Concern

A capability's statement is what every mock providing it claims about the real
harness. It has two grounds: the real harness, for what it does (its docs and
recorded runs), and the user, for which of that is mocked and where a mock
knowingly differs. A claim neither grounds is a mock inventing behaviour; a
claim only one harness makes, stated as everyone's, is a mock copying another's
quirks.

## Decision

- A capability's `statement` (`spec/capabilities/<id>.yaml`) is
  harness-neutral: what differs between providing harnesses (a default, a
  name, a wire format) is left out of it.
- Each part of the statement is stated by every providing harness's cited doc
  sections (`providers.<harness>.docs`, frozen by sha256 in
  `<harness>-mock/snapshots/MANIFEST.yaml`), or, where that harness's docs say
  nothing about it, shown by one of its cited runs
  (`providers.<harness>.runs`: `<harness>-mock/snapshots/runs/<name>/`, whose
  `samples/<ts>/` hold the real harness's payloads, stream and transcript).
- A run never grounds a part its harness's docs contradict.
- Adding or removing a capability, changing its `statement`, or changing any
  line of a cell's `deviations` carries the user's words on its commit
  (`Sloprail-Cites-User`).
