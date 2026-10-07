#!/bin/sh
# Every event logs its raw payload. beforeSubmitPrompt refuses the prompt: continue:false with a
# user message. afterAgentResponse and stop would be fired side by side if the agent ran, so
# the stop script waits (at most 10 s) for its turn's afterAgentResponse, as in runs/tui-stop.
IN=$(cat)
EV=$(printf '%s' "$IN" | jq -r '.hook_event_name')
if [ "$EV" = stop ]; then
  n=0
  while [ "$(grep -c '"hook_event_name":"afterAgentResponse"' "$HOOK_LOG" 2>/dev/null)" -le "$(grep -c '"hook_event_name":"stop"' "$HOOK_LOG" 2>/dev/null)" ] && [ "$n" -lt 200 ]; do
    n=$((n + 1)); sleep 0.05
  done
fi
printf '%s\n' "$IN" >>"$HOOK_LOG"
if [ "$EV" = beforeSubmitPrompt ]; then
  printf '{"continue":false,"user_message":"PROMPT-BLOCK-MSG"}\n'
fi
exit 0
