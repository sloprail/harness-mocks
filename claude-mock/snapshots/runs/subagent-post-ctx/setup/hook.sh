#!/bin/sh
# Every payload is logged; a PostToolUse returns JSON additionalContext, tagged with
# who ran the tool (the payload names the sub-agent inside one).
IN=$(cat)
printf '%s\n' "$IN" >>"$HOOK_LOG"
EV=$(printf '%s' "$IN" | jq -r .hook_event_name)
WHO=$(printf '%s' "$IN" | jq -r '.agent_type // "main"')
case "$EV" in
  PostToolUse)
    printf '{"hookSpecificOutput":{"hookEventName":"PostToolUse","additionalContext":"POST-CTX-%s"}}' "$WHO" ;;
esac
exit 0
