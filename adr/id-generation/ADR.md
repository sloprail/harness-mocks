---
concern: how a mock generates the identifier of a session or of a record
sloprails: [file-guard/adr-conformance]
---

# One helper generates every session and record identifier

## Concern

Every identifier a mock mints for a session, a transcript record or a task, and
what happens when the random source fails.

## Decision

- A mock gets a session or record identifier from `session.NewID`
  (`internal/session`), never from its own generator.
- `session.NewID` returns the random source's error, never an identifier that is
  not random; the caller reports that error and does not substitute another
  identifier.
- The identifier has the shape of a version 4 UUID.
