#!/bin/sh
# Every event logs its payload. The stop hook asks, the first time, for the
# turn to go on: followup_message on its output, and an exit 2 on the second.
IN=$(cat)
printf '%s\n' "$IN" >>"$HOOK_LOG"
EV=$(printf '%s' "$IN" | jq -r '.hook_event_name')
case "$EV" in
  stop)
    N=$(cat "$TMPDIR/stopn" 2>/dev/null || echo 0); N=$((N + 1)); echo $N >"$TMPDIR/stopn"
    if [ $N = 1 ]; then echo '{"followup_message":"STOP-FOLLOWUP-REASON"}'; exit 0; fi
    if [ $N = 2 ]; then echo "STOP-EXIT2-REASON" >&2; exit 2; fi ;;
esac
exit 0
