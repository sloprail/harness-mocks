#!/bin/sh
# Logs which configured hook ran, for which event and tool, and the payload it was given.
IN=$(cat)
EV=$(printf '%s' "$IN" | jq -r '.hook_event_name')
TOOL=$(printf '%s' "$IN" | jq -r '.tool_name // ""')
CMD=$(printf '%s' "$IN" | jq -r '.command // .tool_input.command // ""')
printf '{"hook_ran":"%s","event":"%s","tool":"%s","command":"%s"}\n' "$1" "$EV" "$TOOL" "$CMD" >>"$HOOK_LOG"
printf '%s\n' "$IN" >>"$HOOK_LOG"
exit 0
