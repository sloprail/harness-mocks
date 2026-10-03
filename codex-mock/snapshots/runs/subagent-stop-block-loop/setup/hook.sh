#!/bin/sh
# Every event logs its payload. SubagentStop blocks the first time by JSON
# (SUBSTOP-REASON-1), the second time by exit 2 (SUBSTOP-REASON-2), then lets
# the sub-agent stop.
IN=$(cat)
printf '%s\n' "$IN" >>"$HOOK_LOG"
EV=$(printf '%s' "$IN" | jq -r '.hook_event_name')
case "$EV" in
  SubagentStop)
    N=$(cat "$TMPDIR/substopn" 2>/dev/null || echo 0); N=$((N + 1)); echo $N >"$TMPDIR/substopn"
    if [ $N = 1 ]; then echo '{"decision":"block","reason":"SUBSTOP-REASON-1"}'; exit 0; fi
    if [ $N = 2 ]; then echo "SUBSTOP-REASON-2" >&2; exit 2; fi
    echo '{}' ;;
esac
exit 0
