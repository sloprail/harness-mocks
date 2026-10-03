#!/bin/sh
# Lays out two plugin directories in the scratch repo, p1 and p2 (the args load
# only p1). Each has a beforeShellExecution hook that logs its own name; the
# project has one too (hooks.json). $PWD is the scratch repo.
set -e
for p in p1 p2; do
  mkdir -p "plugins/$p/.cursor-plugin" "plugins/$p/hooks"
  printf '{"name":"%s","version":"1.0.0","description":"plugin %s","hooks":"./hooks/hooks.json"}\n' "$p" "$p" >"plugins/$p/.cursor-plugin/plugin.json"
  cat >"plugins/$p/hooks/hooks.json" <<JSON
{
  "version": 1,
  "hooks": {
    "beforeShellExecution": [{ "command": "cat >/dev/null; printf '{\"hook_ran\":\"$p\"}\n' >>\"\$HOOK_LOG\"" }]
  }
}
JSON
done
