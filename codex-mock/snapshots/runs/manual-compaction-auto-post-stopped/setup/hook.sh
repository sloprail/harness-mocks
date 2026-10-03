#!/bin/sh
# Every event logs its payload; PostCompact says to stop.
IN=$(cat)
printf '%s\n' "$IN" >>"$HOOK_LOG"
case "$IN" in
  *'"PostCompact"'*) printf '{"continue":false,"stopReason":"no more"}\n' ;;
esac
exit 0
