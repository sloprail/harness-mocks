#!/bin/sh
# One hook for every event. The sessionStart hook tries to stop the session the
# way the other hooks do: it prints {"continue": false, ...} and exits 0. Every
# other hook only logs its payload and exits 0, so the log shows whether the
# session went on past the start, and which hooks fired at all.
IN=$(cat)
printf '%s\n' "$IN" >>"$HOOK_LOG"
EV=$(printf '%s' "$IN" | jq -r '.hook_event_name')
if [ "$EV" = sessionStart ]; then
  echo '{"continue":false,"user_message":"SESSION-START-STOPPED"}'
fi
printf '{"hook_result":{"event":"%s","exit":0}}\n' "$EV" >>"$HOOK_LOG"
exit 0
