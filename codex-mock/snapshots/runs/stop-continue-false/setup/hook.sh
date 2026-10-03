#!/bin/sh
# Two Stop hooks run this script. "block" blocks (a block decision with a
# reason) on each of the first two Stop events, then lets the turn end; "halt"
# always answers continue:false with a stop reason. Both log their payload.
IN=$(cat)
printf '%s\n' "$IN" >>"$HOOK_LOG"
case "$1" in
  block)
    N=$(cat "$TMPDIR/stopn" 2>/dev/null || echo 0); N=$((N + 1)); echo $N >"$TMPDIR/stopn"
    printf '{"hook_result":{"hook":"block","call":%s}}\n' "$N" >>"$HOOK_LOG"
    if [ $N -le 2 ]; then echo '{"decision":"block","reason":"BLOCK-REASON"}'; fi ;;
  halt)
    printf '{"hook_result":{"hook":"halt"}}\n' >>"$HOOK_LOG"
    echo '{"continue":false,"stopReason":"HALT-REASON"}' ;;
esac
exit 0
