#!/bin/sh
# Lays out a local plugin marketplace in the scratch repo and registers it in
# the run's config.toml: plugin p1 is enabled, p2 is disabled, and p3 names a
# marketplace the config never declares. Each plugin has a PreToolUse hook that
# logs its own name; the user layer has one too (hooks.json). $PWD is the
# scratch repo; HOME and CODEX_HOME are the run's.
set -e
mkdir -p mk/.agents/plugins
cat >mk/.agents/plugins/marketplace.json <<'JSON'
{
  "name": "mk",
  "interface": { "displayName": "mk" },
  "plugins": [
    { "name": "p1", "source": { "source": "local", "path": "./plugins/p1" }, "policy": { "installation": "AVAILABLE", "authentication": "ON_INSTALL" }, "category": "Test" },
    { "name": "p2", "source": { "source": "local", "path": "./plugins/p2" }, "policy": { "installation": "AVAILABLE", "authentication": "ON_INSTALL" }, "category": "Test" }
  ]
}
JSON
for p in p1 p2; do
  mkdir -p "mk/plugins/$p/.codex-plugin" "mk/plugins/$p/hooks"
  printf '{"name":"%s","version":"1.0.0","description":"plugin %s"}\n' "$p" "$p" >"mk/plugins/$p/.codex-plugin/plugin.json"
  cat >"mk/plugins/$p/hooks/hooks.json" <<JSON
{
  "hooks": {
    "PreToolUse": [
      { "matcher": "Bash", "hooks": [{ "type": "command", "command": "\"\$(git rev-parse --show-toplevel)\"/hook.sh $p" }] }
    ]
  }
}
JSON
done
# `codex exec` does not install a plugin by itself: add the marketplace and
# install both plugins into the run's home; p2 is then disabled, and p3 is
# enabled under a marketplace that was never added.
codex plugin marketplace add "$PWD/mk" >&2
codex plugin add p1@mk >&2
codex plugin add p2@mk >&2
sed -i '' '/^\[plugins."p2@mk"\]/{n;s/true/false/;}' "$CODEX_HOME/config.toml"
printf '\n[plugins."p3@undeclared"]\nenabled = true\n' >>"$CODEX_HOME/config.toml"
