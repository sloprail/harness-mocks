---
status: proposed
---

# ADR-0002: one shared core, harness mocks depend on it and not on each other

**Concern.** Package dependencies between the shared core and the per-harness
mocks. Behaviour common to every harness (tool execution, the session store and
lifecycle, streaming, hook dispatch) exists once. A mock that imports another
mock is the first step to copying one harness's shape into another.

**Decision.**
- `core/**` imports no `*-mock/**` package.
- A `<a>-mock/**` package imports no `<b>-mock/**` package. It may import
  `core/**` and its own packages.

**Consequences.** Anything two mocks both need moves to `core/` first.
