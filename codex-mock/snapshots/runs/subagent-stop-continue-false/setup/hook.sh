#!/bin/sh
# Two SubagentStop hooks run this script. "block" blocks (a block decision with a
# reason) on each of the first two SubagentStop events, then lets the sub-agent stop;
# "halt" always answers continue:false with a stop reason. Both log their payload.
IN=$(cat)
printf '%s\n' "$IN" >>"$HOOK_LOG"
case "$1" in
  block)
    N=$(cat "$TMPDIR/substopn" 2>/dev/null || echo 0); N=$((N + 1)); echo $N >"$TMPDIR/substopn"
    printf '{"hook_result":{"hook":"block","call":%s}}\n' "$N" >>"$HOOK_LOG"
    if [ $N -le 2 ]; then echo '{"decision":"block","reason":"SUBBLOCK-REASON"}'; fi ;;
  halt)
    printf '{"hook_result":{"hook":"halt"}}\n' >>"$HOOK_LOG"
    echo '{"continue":false,"stopReason":"SUBHALT-REASON"}' ;;
esac
exit 0
