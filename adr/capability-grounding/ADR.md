---
concern: "what grounds a capability: the real harness for what it does, the user for what is mocked"
sloprails: [file-guard/capability-grounded, file-guard/capability-rigor]
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
- For a supported cell, each part of the statement is either stated by the
  harness's cited doc sections (`providers.<harness>.docs`, frozen by sha256 in
  `<harness>-mock/snapshots/MANIFEST.yaml`), or shown by one of its cited runs
  (`providers.<harness>.runs`: `<harness>-mock/snapshots/runs/<name>/`, whose
  `samples/<ts>/` hold the real harness's payloads, stream and transcript), or
  listed in the cell's `deviations` as a part the harness does not do.
- A cell `{supported: false, reason, docs?, runs?}` claims the harness lacks
  the behaviour, and is grounded by evidence of the same two kinds, at least
  one: `runs`, recordings whose scenario attempts the behaviour and whose
  samples show the real harness not doing it, and/or `docs` that show the
  feature absent or cover the whole area without it; the judge reads them for
  that. A harness whose recordings show only some parts of the statement is
  supported, and the parts it does not do are its `deviations`. A `pending`
  cell claims nothing and grounds nothing.
- A recording outranks a doc: where a recorded run and a doc sentence
  disagree, what the real harness was recorded doing is what it does. A cell
  is supported when its recordings show the behaviour, whatever a doc says
  elsewhere, and unsupported when its recordings show the behaviour not
  happening, even where a doc describes it. The disagreement is not left
  silent: the cell discloses it as a `deviations` entry that starts "Doc and
  recording conflict:", names the doc section, quotes its sentence and says
  what the recording shows.
- A supported cell's deviations never negate the statement's defining clause,
  the part without which the harness would not be said to provide the
  capability. A cell whose deviations say the harness does not do that part
  is `supported: false`, grounded by the evidence of the absence.
- Adding or removing a capability, or changing its `statement`, carries the
  user's words on its commit (`Sloprail-Cites-User`): the statement is the
  capability the user wants implemented.
- Every `deviations` entry has a `kind`: `mock-not-modeled` (the mock leaves
  out something the real harness does) or `harness-lacks` (the harness itself
  differs from the statement, shown by a cited doc or recording, including the
  "Doc and recording conflict:" entry). The schema requires it. A
  `harness-lacks` deviation cites at least one doc or recorded run in its cell
  (`capability-grounded/harness-lacks-cited.sh`).
- Adding or changing a `mock-not-modeled` deviation carries the user's words
  on its commit (`Sloprail-Cites-User`) like a statement: what a mock does not
  model is the user's call. So does a cell turning `supported: false` without a
  cited recording. A `harness-lacks` deviation, and a `supported: false` cell
  with a recording that shows the harness not doing it, need no quote: the
  recording grounds them. This is decided by a script
  (`capability-grounded/added-or-removed.sh`), not by a judge.
