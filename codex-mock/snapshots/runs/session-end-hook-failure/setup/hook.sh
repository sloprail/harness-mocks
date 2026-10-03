#!/bin/sh
# SessionEnd handlers that fail, one way each: exit 1, exit 2, a timeout (the
# 1 second the config gives it), and an async one that takes half a second:
# its "done" line reaches the log only if the run waited for it.
IN=$(cat)
printf '%s\n' "$IN" >>"$HOOK_LOG"
printf '{"ran":"%s"}\n' "$1" >>"$HOOK_LOG"
case "$1" in
  exit1) echo "SE-EXIT1-STDERR" >&2; exit 1 ;;
  exit2) echo "SE-EXIT2-STDERR" >&2; exit 2 ;;
  slow) sleep 5; printf '{"done":"slow"}\n' >>"$HOOK_LOG" ;;
  async) sleep 0.5; printf '{"done":"async"}\n' >>"$HOOK_LOG" ;;
esac
exit 0
