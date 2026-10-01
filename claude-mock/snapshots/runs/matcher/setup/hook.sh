#!/bin/sh
# Every matcher group runs this with its own name as the argument. It logs the
# group that fired and the payload's event and tool, so a run shows which
# matchers selected which events.
IN=$(cat)
EV=$(printf '%s' "$IN" | jq -r '.hook_event_name')
TOOL=$(printf '%s' "$IN" | jq -r '.tool_name // ""')
printf '{"matched_group":"%s","event":"%s","tool":"%s"}\n' "$1" "$EV" "$TOOL" >>"$HOOK_LOG"
exit 0
