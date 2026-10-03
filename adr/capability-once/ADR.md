---
concern: where a mocked harness capability is implemented, and how it is proven
sloprails: [file-guard/shapes, file-guard/capability-covered, file-guard/capability-rigor, file-guard/adr-conformance]
---

# Each mocked capability is implemented once, in core

## Concern

Harness behaviour the mocks model: a Stop hook that blocks the turn, a
PreToolUse refusal, compaction, background tasks, sub-agents. Several harnesses
provide the same capability with different wire formats.

## Decision

- `spec/capabilities/<id>.yaml` holds each capability the mocks model: a
  harness-neutral statement, and for every harness mock one of: the doc
  sections and recorded runs of that harness that show it (supported);
  `{supported: false, reason, docs}`, the docs being the sections that show the
  feature absent or cover the area without it (a bare `false` is not a cell:
  absence needs evidence); or `pending`: the harness mock has no adapter for
  it. A `pending` cell does not block a merge and is never coverage;
  `file-guard/capability-covered` lists every one on each run.
- A capability's `<id>`, and so its file name, is kebab-case.
- A capability's behaviour lives once, in `internal/`, on code marked
  `// sr:capability <id>`.
- A harness mock holds only its adapter: that harness's event names, payload
  encoding and flags for the capability. Adapter code is marked
  `// sr:provides <id>/<harness>`, and exists exactly for the supported
  cells; a `pending` or unsupported cell has none. Any code under `<harness>-mock/` that affects a capability's
  behaviour is that capability's adapter, marked or not.
- An adapter translates and decides nothing: when a hook fires, whether a
  block is honoured, and in what order things happen are the capability's.
- Behaviour that differs between harnesses is a parameter of the core
  capability, never a copy of it.
- Every supported cell has tests, in `*_test.go`, marked
  `// sr:proves <id>/<harness>`; the marker sits on tests only.
- Those tests assert what the cell's recorded runs and docs show the real
  harness doing, or the cell's declared deviation where the mock differs.
