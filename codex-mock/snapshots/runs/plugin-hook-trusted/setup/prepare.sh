#!/bin/sh
# Lays out a local plugin marketplace mk with one plugin, p1, whose PreToolUse hook logs what it is told of
# its plugin, and installs it into the run's home with `codex plugin`. $PWD is the scratch repo.
set -e
mkdir -p mk/.agents/plugins mk/plugins/p1/.codex-plugin mk/plugins/p1/hooks
cat >mk/.agents/plugins/marketplace.json <<'JSON'
{
  "name": "mk",
  "interface": { "displayName": "mk" },
  "plugins": [
    { "name": "p1", "source": { "source": "local", "path": "./plugins/p1" }, "policy": { "installation": "AVAILABLE", "authentication": "ON_INSTALL" }, "category": "Test" }
  ]
}
JSON
printf '{"name":"p1","version":"1.0.0","description":"plugin p1"}\n' >mk/plugins/p1/.codex-plugin/plugin.json
cat >mk/plugins/p1/hooks/hooks.json <<'JSON'
{
  "hooks": {
    "PreToolUse": [
      { "matcher": "Bash", "hooks": [{ "type": "command", "command": "\"$(git rev-parse --show-toplevel)\"/hook.sh p1" }] }
    ]
  }
}
JSON
codex plugin marketplace add "$PWD/mk" >&2
codex plugin add p1@mk >&2
