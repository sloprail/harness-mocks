#!/bin/sh
# Every payload is logged; a SessionStart returns JSON additionalContext, tagged with
# its source (startup / resume), so which start added which text is readable.
IN=$(cat)
printf '%s\n' "$IN" >>"$HOOK_LOG"
EV=$(printf '%s' "$IN" | jq -r .hook_event_name)
SRC=$(printf '%s' "$IN" | jq -r '.source // "none"')
case "$EV" in
  SessionStart)
    printf '{"hookSpecificOutput":{"hookEventName":"SessionStart","additionalContext":"START-CTX-%s"}}' "$SRC" ;;
esac
exit 0
