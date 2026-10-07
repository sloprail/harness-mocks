#!/bin/sh
# Logs which hook ran (the argument names it), and the NAMES (never the values) of the variables a plugin's
# hook is told of its plugin: those named PLUGIN_ROOT, PLUGIN_DATA, or those two under CLAUDE_.
cat >/dev/null
printf '{"hook_ran":"%s","plugin_env_names":"%s"}\n' "$1" "$(env | cut -d= -f1 | grep -E '^(CLAUDE_)?PLUGIN_' | sort | tr '\n' ' ')" >>"$HOOK_LOG"
exit 0
