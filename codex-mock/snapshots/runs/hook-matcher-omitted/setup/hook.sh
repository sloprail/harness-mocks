#!/bin/sh
# Every hook says which handler it is and for which event it ran.
IN=$(cat)
ev=$(printf '%s' "$IN" | jq -r '.hook_event_name')
tool=$(printf '%s' "$IN" | jq -r '.tool_name // ""')
printf '{"ran":"%s","event":"%s","tool":"%s"}\n' "$1" "$ev" "$tool" >>"$HOOK_LOG"
exit 0
