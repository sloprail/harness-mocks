#!/bin/sh
IN=$(cat); printf '%s\n' "$IN" >> "$(dirname "$0")/payloads.jsonl"
EV=$(printf '%s' "$IN" | sed -n 's/.*"hook_event_name":"\([A-Za-z]*\)".*/\1/p')
case "$EV" in Stop|SubagentStop) echo '{"decision":"block","reason":"KEEP GOING"}';; esac
exit 0
