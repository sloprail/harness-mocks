#!/bin/sh
# Every event logs its payload; Stop and SubagentStop always block.
IN=$(cat)
printf '%s\n' "$IN" >>"$HOOK_LOG"
EV=$(printf '%s' "$IN" | jq -r '.hook_event_name')
case "$EV" in Stop | SubagentStop) echo '{"decision":"block","reason":"KEEP GOING"}' ;; esac
exit 0
