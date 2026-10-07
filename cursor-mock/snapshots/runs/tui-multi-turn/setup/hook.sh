#!/bin/sh
# Every event logs its raw payload; none answers. afterAgentResponse and stop of one turn are
# fired side by side, so which of the two scripts reaches the log first is a race: the stop script
# waits (at most 10 s) for its turn's afterAgentResponse to be logged, so every recording has them
# in the order the turn happens. The payloads themselves are not touched.
IN=$(cat)
EV=$(printf '%s' "$IN" | jq -r '.hook_event_name')
if [ "$EV" = stop ]; then
  n=0
  while [ "$(grep -c '"hook_event_name":"afterAgentResponse"' "$HOOK_LOG" 2>/dev/null)" -le "$(grep -c '"hook_event_name":"stop"' "$HOOK_LOG" 2>/dev/null)" ] && [ "$n" -lt 200 ]; do
    n=$((n + 1)); sleep 0.05
  done
fi
printf '%s\n' "$IN" >>"$HOOK_LOG"
exit 0
