#!/bin/sh
# Every handler logs its payload and which handler it is. The SessionEnd
# handlers PRINT: plain text, JSON additionalContext and a JSON systemMessage,
# so a run shows whether anything a SessionEnd hook prints reaches the
# transcript, the event stream or stdout.
IN=$(cat)
printf '%s\n' "$IN" >>"$HOOK_LOG"
printf '{"ran":"%s"}\n' "$1" >>"$HOOK_LOG"
case "$1" in
  plain) echo "SE-PLAIN-OUT" ;;
  context) echo '{"hookSpecificOutput":{"hookEventName":"SessionEnd","additionalContext":"SE-CONTEXT-OUT"}}' ;;
  message) echo '{"systemMessage":"SE-MESSAGE-OUT"}' ;;
esac
exit 0
