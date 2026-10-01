#!/bin/sh
# One hook for every event. A beforeShellExecution or preToolUse hook prints a
# valid JSON deny and exits with a non-zero status other than 2:
#   JSONEXIT1  prints {"permission":"deny",...} and exits 1
#   JSONEXIT3  prints {"permission":"deny",...} and exits 3
IN=$(cat)
printf '%s\n' "$IN" >>"$HOOK_LOG"
EV=$(printf '%s' "$IN" | jq -r '.hook_event_name')
code=0
case "$EV" in
  beforeShellExecution|preToolUse)
    if printf '%s' "$IN" | grep -q JSONEXIT1; then echo "{\"permission\":\"deny\",\"user_message\":\"JSON-DENY-EXIT1-$EV\"}"; code=1; fi
    if printf '%s' "$IN" | grep -q JSONEXIT3; then echo "{\"permission\":\"deny\",\"user_message\":\"JSON-DENY-EXIT3-$EV\"}"; code=3; fi ;;
esac
printf '{"hook_result":{"event":"%s","exit":%s}}\n' "$EV" "$code" >>"$HOOK_LOG"
exit $code
