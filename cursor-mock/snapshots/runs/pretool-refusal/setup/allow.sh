#!/bin/sh
# The second preToolUse hook: it allows every call, and says so in the log.
IN=$(cat)
EV=$(printf '%s' "$IN" | jq -r '.hook_event_name')
CMD=$(printf '%s' "$IN" | jq -r '.tool_input.command // ""')
printf '{"hook_result":{"script":"allow","event":"%s","command":"%s"}}\n' "$EV" "$CMD" >>"$HOOK_LOG"
echo '{"permission":"allow"}'
exit 0
