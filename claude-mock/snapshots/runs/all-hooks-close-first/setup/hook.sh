#!/bin/sh
# As all-hooks, but A answers 100 ms after it starts and B after 25 ms: the two blockers finish
# about 75 ms apart, A last. The block acted on shows whether the harness acts on the last to
# finish (A's) or on the last configured (B's).
IN=$(cat)
printf '{"hook_ran":"%s"}\n' "$1" >>"$HOOK_LOG"
case "$1" in
  A) sleep 0.1; echo "BLOCK-A" >&2; exit 2 ;;
  B) sleep 0.025; echo "BLOCK-B" >&2; exit 2 ;;
esac
exit 0
