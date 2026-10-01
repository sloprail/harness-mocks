---
concern: the shape of the project's structured files, which other rules read and trust
sloprails: [gate/shapes, file-guard/shapes]
---

# Every structured file has a schema, checked before it lands and at commit

## Concern

Rules across the project read structured files and trust their shape:
invariants, capabilities, ADR frontmatter, a module's `module.yaml`, and each
harness snapshot's `MANIFEST.yaml` and `run.yaml`. A rule that reads a broken
shape misjudges, or refuses for the wrong reason.

## Decision

- Each kind of structured file has one CUE schema under `.sloprail/schemas/`,
  with closed definitions: a key the schema does not name is an error.
- A write that would land a file breaking its schema is refused before it
  lands, except snapshot files, which only `capture.sh` writes.
- At commit, every structured file the change touches matches its schema,
  snapshot files included: the backstop for writes the first check cannot
  predict.
- A new kind of structured file gets its schema, and its place in both checks,
  in the change that introduces it.
