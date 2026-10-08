#!/bin/sh
# Lays out four plugins, each with hooks that log the event, which plugin it is, the directory the hook
# runs in and whether it was given a CURSOR_PLUGIN_ROOT (its value is the same for hooks that run
# side by side and is not logged):
#   p1  a directory in the user's local plugins, ~/.cursor/plugins/local/p1 (the manifest names its hooks file)
#   p2  a symlink there, ~/.cursor/plugins/local/p2, to a plugin outside that directory (the layout
#       an installer that links a plugin in makes; the manifest names only the plugin, so the hooks
#       file is the default hooks/hooks.json)
#   p3  a directory of the scratch repo that the args load with --plugin-dir
#   p4  a symlink in the local plugins to a plugin kept inside that directory, under a dot-name
# $HOME is the run's, $PWD the scratch repo.
set -e
plugin() { # <dir> <name> <manifest json>
  mkdir -p "$1/.cursor-plugin" "$1/hooks"
  printf '%s\n' "$3" >"$1/.cursor-plugin/plugin.json"
  cat >"$1/hooks/hooks.json" <<JSON
{
  "version": 1,
  "hooks": {
    "sessionStart": [{ "command": "./hooks/log.sh $2" }],
    "sessionEnd": [{ "command": "./hooks/log.sh $2" }],
    "beforeSubmitPrompt": [{ "command": "./hooks/log.sh $2" }],
    "beforeShellExecution": [{ "command": "./hooks/log.sh $2" }],
    "afterAgentResponse": [{ "command": "./hooks/log.sh $2" }],
    "stop": [{ "command": "./hooks/log.sh $2" }]
  }
}
JSON
  cat >"$1/hooks/log.sh" <<'SH'
#!/bin/sh
EV=$(cat | jq -r '.hook_event_name')
SET=false; [ -n "$CURSOR_PLUGIN_ROOT" ] && SET=true
printf '{"hook_result":{"event":"%s","hook":"%s","ran_in":"%s","plugin_root_set":%s}}\n' "$EV" "$1" "$PWD" "$SET" >>"$HOOK_LOG"
exit 0
SH
  chmod +x "$1/hooks/log.sh"
}
plugin "$HOME/.cursor/plugins/local/p1" p1 '{"name":"p1","version":"1.0.0","description":"plugin p1","hooks":"./hooks/hooks.json"}'
plugin "$HOME/outside/p2" p2 '{"name":"p2"}'
ln -s "$HOME/outside/p2" "$HOME/.cursor/plugins/local/p2"
plugin plugins/p3 p3 '{"name":"p3","version":"1.0.0","description":"plugin p3","hooks":"./hooks/hooks.json"}'
plugin "$HOME/.cursor/plugins/local/.store/p4" p4 '{"name":"p4"}'
ln -s "$HOME/.cursor/plugins/local/.store/p4" "$HOME/.cursor/plugins/local/p4"
