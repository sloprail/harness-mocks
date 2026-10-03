#!/bin/sh
# Every event logs its payload. The stop hook, the first time it fires, asks the
# agent to continue (followup_message); the second time it lets the turn end.
IN=$(cat)
printf '%s\n' "$IN" >>"$HOOK_LOG"
EV=$(printf '%s' "$IN" | jq -r '.hook_event_name')
if [ "$EV" = stop ]; then
  N=$(cat "$TMPDIR/stopn" 2>/dev/null || echo 0); N=$((N + 1)); echo $N >"$TMPDIR/stopn"
  if [ $N = 1 ]; then echo '{"followup_message":"Now reply only DONE2."}'; fi
fi
exit 0
