#!/bin/sh
# Logs each payload, then the directory this hook itself ran in.
IN=$(cat)
printf '%s\n' "$IN" >>"$HOOK_LOG"
printf '{"hook_pwd":"%s","event":"%s"}\n' "$(pwd -P)" "$(printf '%s' "$IN" | jq -r '.hook_event_name')" >>"$HOOK_LOG"
exit 0
