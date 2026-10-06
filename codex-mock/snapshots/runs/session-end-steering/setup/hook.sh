#!/bin/sh
# SessionEnd hooks that try to steer the run: continue:false, and a block decision with a reason.
IN=$(cat)
printf '%s\n{"ran":"%s"}\n' "$IN" "$1" >>"$HOOK_LOG" # one write: the hooks run at once
case "$1" in
  stop) echo '{"continue":false,"stopReason":"SE-STOPREASON"}' ;;
  block) echo '{"decision":"block","reason":"SE-BLOCK-REASON"}' ;;
esac
exit 0
