#!/bin/sh
# Installs two plugins from a local marketplace mk with `codex plugin`, as a user does, and upgrades one:
# p1 (version 1.0.0, then 1.1.0 with a file added, one removed and a symbolic link, dotfiles) and p2 (a manifest
# with no version). Each plugin's PreToolUse hook is a script of the plugin, run by the PLUGIN_ROOT it is told,
# and logs that root and the files found under it. $PWD is the scratch repo.
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
  mkdir -p "mk/plugins/$p/.codex-plugin" "mk/plugins/$p/hooks" "mk/plugins/$p/scripts" "mk/plugins/$p/.hidden"
  cat >"mk/plugins/$p/hooks/hooks.json" <<'JSON'
{
  "hooks": {
    "PreToolUse": [
      { "matcher": "Bash", "hooks": [{ "type": "command", "command": "sh \"$PLUGIN_ROOT/hooks/log.sh\"" }] }
    ]
  }
}
JSON
  cat >"mk/plugins/$p/hooks/log.sh" <<SH
cat >/dev/null
printf '{"hook_ran":"$p","root":"%s","files":"%s"}\n' "\$PLUGIN_ROOT" "\$(cd "\$PLUGIN_ROOT" && find . | sort | tr '\n' ' ')" >>"\$HOOK_LOG"
SH
  echo run >"mk/plugins/$p/scripts/run.sh"
  echo hidden >"mk/plugins/$p/.hidden/h"
  echo readme >"mk/plugins/$p/README.md"
  ln -s README.md "mk/plugins/$p/link"
done
printf '{"name":"p1","version":"1.0.0"}\n' >mk/plugins/p1/.codex-plugin/plugin.json
printf '{"name":"p2"}\n' >mk/plugins/p2/.codex-plugin/plugin.json
codex plugin marketplace add "$PWD/mk" >&2
codex plugin add p1@mk >&2
codex plugin add p2@mk >&2
printf '{"name":"p1","version":"1.1.0"}\n' >mk/plugins/p1/.codex-plugin/plugin.json
echo new >mk/plugins/p1/scripts/new.sh
rm mk/plugins/p1/README.md
codex plugin add p1@mk >&2
