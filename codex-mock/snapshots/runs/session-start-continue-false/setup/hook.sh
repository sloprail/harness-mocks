#!/bin/sh
# SessionStart tries to stop the session with continue:false; every event
# logs its payload, so the log shows what still happened after it.
IN=$(cat)
printf '%s\n' "$IN" >>"$HOOK_LOG"
ev=$(printf '%s' "$IN" | jq -r '.hook_event_name')
if [ "$ev" = SessionStart ]; then
  echo '{"continue":false,"stopReason":"SS-STOP-REASON"}'
fi
exit 0
