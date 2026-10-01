#!/bin/sh
# Two hooks per event (the argument names which). UserPromptSubmit and PostToolUse
# each return JSON additionalContext, tagged with the argument. Every payload is
# logged.
IN=$(cat)
printf '%s\n' "$IN" >>"$HOOK_LOG"
EV=$(printf '%s' "$IN" | jq -r .hook_event_name)
case "$EV" in
  UserPromptSubmit | PostToolUse)
    printf '{"hookSpecificOutput":{"hookEventName":"%s","additionalContext":"%s-CTX-%s"}}' "$EV" "$EV" "$1" ;;
esac
exit 0
