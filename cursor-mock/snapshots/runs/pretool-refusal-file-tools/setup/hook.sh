#!/bin/sh
# Every event logs its payload and the exit status this script gave it.
# sessionStart makes the two files the prompt names (a hook runs from the
# project root). preToolUse refuses a Write of protected.txt by JSON and a Read
# of secret.txt by exit 2. sessionEnd logs what protected.txt holds by then.
IN=$(cat)
printf '%s\n' "$IN" >>"$HOOK_LOG"
EV=$(printf '%s' "$IN" | jq -r '.hook_event_name')
TOOL=$(printf '%s' "$IN" | jq -r '.tool_name // ""')
FILE=$(printf '%s' "$IN" | jq -r '.tool_input.file_path // ""')
code=0
case "$EV" in
  sessionStart)
    echo ORIGINAL >protected.txt
    echo SECRET >secret.txt ;;
  preToolUse)
    if [ "$TOOL" = Write ] && [ "$(basename "$FILE")" = protected.txt ]; then
      echo '{"permission":"deny","user_message":"WRITE-DENY-MSG","agent_message":"WRITE-AGENT-MSG"}'
    fi
    if [ "$TOOL" = Read ] && [ "$(basename "$FILE")" = secret.txt ]; then
      echo "READ-BLOCK-MSG" >&2
      code=2
    fi ;;
  sessionEnd)
    printf '{"hook_result":{"event":"files","protected":"%s"}}\n' "$(cat protected.txt)" >>"$HOOK_LOG" ;;
esac
printf '{"hook_result":{"event":"%s","exit":%s}}\n' "$EV" "$code" >>"$HOOK_LOG"
exit $code
