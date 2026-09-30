---
concern: "hooks: settings, matching, running handlers and interpreting their results"
sloprails: [file-guard/module-boundaries, file-guard/concern-placement]
home: ["core/hooks/**"]
api: ["core/hooks"]
---

# The hooks module owns everything about hooks

## Concern

Hooks: reading hook settings (project, user, plugin), matching an event to its
handlers, running a handler command, and interpreting its exit code and JSON
output (block, allow, additional context).

## Decision

- Everything about hooks lives under `core/hooks/`: settings, matching,
  invocation, and the meaning of a handler's result.
- Other code fires a hook only through the `core/hooks` package's API, and
  acts on the typed outcome it returns. It never reads a handler's exit code or
  stdout itself.
- Packages under `core/hooks/` other than `core/hooks` itself are internal to
  the module: nothing outside it imports them.
