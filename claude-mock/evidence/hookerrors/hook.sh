#!/bin/sh
IN=$(cat); printf '%s\n' "$IN" >> "$(dirname "$0")/payloads.jsonl"
EV=$(printf '%s' "$IN" | sed -n 's/.*"hook_event_name":"\([A-Za-z]*\)".*/\1/p')
case "$EV" in
 SessionStart) echo "SS-BLOCK-MSG" >&2; exit 2;;
 SubagentStart) echo "SAS-BLOCK-MSG" >&2; exit 2;;
 UserPromptSubmit) echo "UPS-PLAIN-OUT";;
 Stop) exit 1;;
 SessionEnd) echo "SE-OUT"; echo "SE-ERR" >&2;;
esac
exit 0
