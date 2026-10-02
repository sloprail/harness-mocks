---
concern: "what grounds a capability: the real harness for what it does, the user for what is mocked"
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
- Adding or removing a capability, changing its `statement`, or adding or
  dropping a harness's cell (a `providers.<harness>` key, or flipping it
  between `false` and a cell) carries the user's words on its commit
  (`Sloprail-Cites-User`): what is mocked is the user's choice.
- A change solely inside a harness's cell of the support matrix (its `docs`,
  `runs` and `deviations`) needs no citation: the cell is held to the harness
  by the rules above, not to the user. Every `deviations` entry still names an
  existing ADR, and a judge refuses one that does not fit the scope of the
  ADR it names, so the matrix cannot declare a gap no ADR allows. A commit
  that touches the matrix together with anything above needs the words as a
  whole.
