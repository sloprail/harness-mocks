#!/bin/sh
# Lays out a local plugin marketplace in the scratch repo and a settings.json
# that declares it: plugin p1 is enabled, p2 is disabled, and p3 names a
# marketplace the settings never declare. Each plugin has a PreToolUse hook that
# logs its own name; the project has one too. $PWD is the scratch repo.
set -e
mkdir -p mk/.claude-plugin
cat >mk/.claude-plugin/marketplace.json <<'EOF'
{
  "name": "mk",
  "owner": { "name": "Test" },
  "plugins": [
    { "name": "p1", "source": "./plugins/p1", "description": "enabled" },
    { "name": "p2", "source": "./plugins/p2", "description": "disabled" }
  ]
}
EOF
for p in p1 p2; do
  mkdir -p "mk/plugins/$p/.claude-plugin" "mk/plugins/$p/hooks"
  printf '{"name":"%s","description":"plugin %s","version":"1.0.0"}\n' "$p" "$p" >"mk/plugins/$p/.claude-plugin/plugin.json"
  cat >"mk/plugins/$p/hooks/hooks.json" <<EOF
{
  "hooks": {
    "PreToolUse": [
      { "matcher": "Bash", "hooks": [{ "type": "command", "command": "\"\${CLAUDE_PROJECT_DIR}\"/hook.sh $p" }] }
    ]
  }
}
EOF
done
cat >.claude/settings.json <<EOF
{
  "extraKnownMarketplaces": { "mk": { "source": { "source": "directory", "path": "$PWD/mk" } } },
  "enabledPlugins": { "p1@mk": true, "p2@mk": false, "p3@undeclared": true },
  "hooks": {
    "PreToolUse": [
      { "matcher": "Bash", "hooks": [{ "type": "command", "command": "\"\$CLAUDE_PROJECT_DIR\"/hook.sh project" }] }
    ]
  }
}
EOF
# `claude -p` does not install a plugin by itself: install both from the local
# marketplace into the hermetic home (the HOME capture.sh gives this script);
# p2 stays disabled because the project's settings say so.
claude plugin marketplace add "$PWD/mk" >&2
claude plugin install p1@mk >&2
claude plugin install p2@mk >&2
