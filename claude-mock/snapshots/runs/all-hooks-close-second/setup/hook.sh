#!/bin/sh
# As all-hooks, but A answers 25 ms after it starts and B after 100 ms: the two blockers finish
# about 75 ms apart, B last. With close-first it tells the block acted on is the last to finish,
# not the first or last configured.
IN=$(cat)
printf '{"hook_ran":"%s"}\n' "$1" >>"$HOOK_LOG"
case "$1" in
  A) sleep 0.025; echo "BLOCK-A" >&2; exit 2 ;;
  B) sleep 0.1; echo "BLOCK-B" >&2; exit 2 ;;
esac
exit 0
