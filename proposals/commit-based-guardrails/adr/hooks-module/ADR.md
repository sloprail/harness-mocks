---
concern: "hooks: settings, matching, running handlers and interpreting their results"
sloprails: [file-guard/module-boundaries, file-guard/module-leaks]
modules: [core/hooks]       # its boundary: core/hooks/module.yaml (home, api)
---

# The hooks module owns everything about hooks

## Concern

Hooks: reading hook settings (project, user, plugin), matching an event to its
handlers, running a handler command, and interpreting its exit code and JSON
output (block, allow, additional context).

## Decision

- Everything about hooks lives in the `core/hooks` module's home: settings,
  matching, invocation, and the meaning of a handler's result.
- Other code fires a hook only through the `core/hooks` package's API, and
  acts on the typed outcome it returns. It never reads a handler's exit code or
  stdout itself.
- Only the module's api package is imported from outside it.
