#!/bin/sh
# The project's own hook: it logs the raw payload. For the recording's sake the local plugins are
# loaded in the background some seconds after the session starts (a prompt typed at once runs
# without them), so sessionStart takes 12 s, and the prompt is typed when it has finished. The
# payloads are not touched.
IN=$(cat)
EV=$(printf '%s' "$IN" | jq -r '.hook_event_name')
[ "$EV" = sessionStart ] && sleep 12
printf '%s\n' "$IN" >>"$HOOK_LOG"
exit 0
