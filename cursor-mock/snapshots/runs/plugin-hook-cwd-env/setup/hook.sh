#!/bin/sh
# The project's own hook: logs the directory it runs in and the CURSOR_PLUGIN_ROOT it is given.
cat >/dev/null
printf '{"hook_result":{"event":"cwd_env","hook":"%s","ran_in":"%s","plugin_root":"%s"}}\n' "$1" "$PWD" "$CURSOR_PLUGIN_ROOT" >>"$HOOK_LOG"
exit 0
