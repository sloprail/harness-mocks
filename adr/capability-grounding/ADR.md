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
- Each part of the statement is stated by every providing harness's cited doc
  sections (`providers.<harness>.docs`, frozen by sha256 in
  `<harness>-mock/snapshots/MANIFEST.yaml`), or, where that harness's docs say
  nothing about it, shown by one of its cited runs
  (`providers.<harness>.runs`: `<harness>-mock/snapshots/runs/<name>/`, whose
  `samples/<ts>/` hold the real harness's payloads, stream and transcript).
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
  happening, even where a doc describes it.
- Adding or removing a capability, or changing its `statement`, carries the
  user's words on its commit (`Sloprail-Cites-User`): the statement is the
  capability the user wants implemented.
- A cell's `deviations` need no user words: they are found empirically or
  follow from other ADRs (e.g. a mock does not wait out a 5-minute background
  limit, so e2e stays fast).
