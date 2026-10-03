#!/bin/sh
# Every event logs its payload; stop always asks the agent to go on.
IN=$(cat)
printf '%s\n' "$IN" >>"$HOOK_LOG"
EV=$(printf '%s' "$IN" | jq -r '.hook_event_name')
case "$EV" in stop) echo '{"followup_message":"KEEP GOING"}' ;; esac
exit 0
