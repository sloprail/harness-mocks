---
concern: which packages core and the harness mocks may import
sloprails: [file-guard/layering, file-guard/adr-conformance]
---

# One shared core; harness mocks depend on it, never on each other

## Concern

Package imports between `internal/` and the per-harness mocks.

## Decision

- No package under `internal/` imports a package under any `*-mock/`.
- No package under `<a>-mock/` imports a package under a different
  `<b>-mock/`. It imports only `internal/` and its own packages.
- Code two mocks both need lives in the repo-root `internal/`.
- A harness's adapter (its wire format, record shapes, CLI) lives in its own
  mock, under `<harness>-mock/internal/`.
- No package under `internal/` holds more than 20 Go files.
