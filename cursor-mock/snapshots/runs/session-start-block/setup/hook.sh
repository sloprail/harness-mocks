#!/bin/sh
# One hook for every event. The sessionStart hook tries to stop the session: it
# says so on stderr and exits 2, the status that blocks any other action. Every
# other hook only logs its payload and exits 0, so the log shows whether the
# session went on past the start.
IN=$(cat)
printf '%s\n' "$IN" >>"$HOOK_LOG"
EV=$(printf '%s' "$IN" | jq -r '.hook_event_name')
code=0
if [ "$EV" = sessionStart ]; then
  echo "SESSION-START-BLOCKED" >&2
  code=2
fi
printf '{"hook_result":{"event":"%s","exit":%s}}\n' "$EV" "$code" >>"$HOOK_LOG"
exit $code
