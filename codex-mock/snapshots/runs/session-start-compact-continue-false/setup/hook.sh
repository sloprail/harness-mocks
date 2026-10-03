#!/bin/sh
# Every event logs its payload; the SessionStart that follows a compaction
# says to stop with continue:false (the first one, at startup, does not).
IN=$(cat)
printf '%s\n' "$IN" >>"$HOOK_LOG"
case "$IN" in
  *'"source":"compact"'*) printf '{"continue":false,"stopReason":"SS-COMPACT-STOP"}\n' ;;
esac
exit 0
