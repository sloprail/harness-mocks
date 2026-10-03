#!/bin/sh
# Every event logs its payload; Stop blocks every time, for its first 30 calls
# (more than any block cap a harness is known to have), then lets the turn end.
IN=$(cat)
printf '%s\n' "$IN" >>"$HOOK_LOG"
EV=$(printf '%s' "$IN" | jq -r '.hook_event_name')
case "$EV" in
  Stop)
    N=$(cat "$TMPDIR/stopn" 2>/dev/null || echo 0); N=$((N + 1)); echo $N >"$TMPDIR/stopn"
    if [ $N -le 30 ]; then echo '{"decision":"block","reason":"KEEP GOING"}'; fi ;;
esac
exit 0
