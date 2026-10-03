#!/bin/sh
# Logs every payload and prints on stdout at SessionStart and SessionEnd, so the
# run shows whether a hook's output reaches the run's stdout (it must not: the
# run prints only the final agent message there).
IN=$(cat)
printf '%s\n' "$IN" >>"$HOOK_LOG"
ev=$(printf '%s' "$IN" | jq -r '.hook_event_name')
case "$ev" in
  SessionStart) echo "HOOK-OUT-START" ;;
  SessionEnd) echo "HOOK-OUT-END" ;;
esac
exit 0
