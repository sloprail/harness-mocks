#!/bin/sh
# Three hooks on one PreToolUse matcher, each with its own name as the argument.
# A and B both block (exit 2, each with its own message); C only logs. Every
# one logs its name, so a run shows that all of them ran, and the refusal the
# agent gets shows which block was acted on.
IN=$(cat)
printf '{"hook_ran":"%s"}\n' "$1" >>"$HOOK_LOG"
case "$1" in
  A) echo "BLOCK-A" >&2; exit 2 ;;
  B) echo "BLOCK-B" >&2; exit 2 ;;
esac
exit 0
