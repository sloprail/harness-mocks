#!/bin/sh
# The project's own hook for every event: it logs the raw payload. Two things are for the recording's
# sake: the local plugins are loaded in the background some seconds after the session starts (a
# prompt typed at once runs without them), so sessionStart takes 12 s, and the prompt is typed when
# it has finished; and stop waits for the turn's afterAgentResponse to be logged (the two are fired
# side by side, so which script reaches the log first is a race). The payloads are not touched.
IN=$(cat)
EV=$(printf '%s' "$IN" | jq -r '.hook_event_name')
[ "$EV" = sessionStart ] && sleep 12
if [ "$EV" = stop ]; then
  n=0
  while [ "$(grep -c '"hook_event_name":"afterAgentResponse"' "$HOOK_LOG" 2>/dev/null)" -lt 1 ] && [ "$n" -lt 200 ]; do
    n=$((n + 1)); sleep 0.05
  done
fi
printf '%s\n' "$IN" >>"$HOOK_LOG"
exit 0
