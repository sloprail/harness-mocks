#!/bin/sh
# As all-hooks, but A takes a second to answer and B answers at once: if the
# block acted on is the last to finish it is A's, if it is the last (or the
# first) in the settings' order it is B's (or A's) whatever the timing.
IN=$(cat)
printf '{"hook_ran":"%s"}\n' "$1" >>"$HOOK_LOG"
case "$1" in
  A) sleep 1; echo "BLOCK-A" >&2; exit 2 ;;
  B) echo "BLOCK-B" >&2; exit 2 ;;
esac
exit 0
