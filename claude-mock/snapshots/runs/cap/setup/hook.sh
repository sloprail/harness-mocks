#!/bin/sh
# Every event logs its payload; Stop and SubagentStop always block, so the
# harness's block cap is what ends the turn.
IN=$(cat); printf '%s\n' "$IN" >> "$HOOK_LOG"
EV=$(printf '%s' "$IN" | sed -n 's/.*"hook_event_name":"\([A-Za-z]*\)".*/\1/p')
case "$EV" in Stop|SubagentStop) echo '{"decision":"block","reason":"KEEP GOING"}';; esac
exit 0
