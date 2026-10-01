#!/bin/sh
# Every event logs its payload; SubagentStop always blocks.
IN=$(cat)
printf '%s\n' "$IN" >>"$HOOK_LOG"
EV=$(printf '%s' "$IN" | jq -r '.hook_event_name')
case "$EV" in SubagentStop) echo '{"decision":"block","reason":"KEEP GOING"}' ;; esac
exit 0
