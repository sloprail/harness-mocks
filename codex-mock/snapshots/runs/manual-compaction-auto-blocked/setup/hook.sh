#!/bin/sh
# Every event logs its payload; PreCompact stops the compaction.
IN=$(cat)
printf '%s\n' "$IN" >>"$HOOK_LOG"
case "$IN" in
  *'"PreCompact"'*) printf '{"continue":false,"stopReason":"no compaction"}\n' ;;
esac
exit 0
