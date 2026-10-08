#!/bin/sh
# Lays out one plugin, p1 (the args load it), in the scratch repo. Its beforeShellExecution hook is
# a command relative to the plugin ("./hooks/log.sh"), which logs the directory it runs in and the
# CURSOR_PLUGIN_ROOT it is given; the project's own hook logs the same, to compare. $PWD is the
# scratch repo.
set -e
mkdir -p plugins/p1/.cursor-plugin plugins/p1/hooks
printf '{"name":"p1","version":"1.0.0","description":"plugin p1","hooks":"./hooks/hooks.json"}\n' >plugins/p1/.cursor-plugin/plugin.json
cat >plugins/p1/hooks/hooks.json <<'JSON'
{
  "version": 1,
  "hooks": {
    "beforeShellExecution": [{ "command": "./hooks/log.sh plugin" }]
  }
}
JSON
cat >plugins/p1/hooks/log.sh <<'SH'
#!/bin/sh
cat >/dev/null
printf '{"hook_result":{"event":"cwd_env","hook":"%s","ran_in":"%s","plugin_root":"%s"}}\n' "$1" "$PWD" "$CURSOR_PLUGIN_ROOT" >>"$HOOK_LOG"
exit 0
SH
chmod +x plugins/p1/hooks/log.sh
