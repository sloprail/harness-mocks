#!/bin/sh
# Two PreToolUse hooks both deny the call, each with its own reason; "slow"
# takes two seconds before it answers, "fast" answers at once. Each logs, when
# it has finished, that it ran.
cat >/dev/null
[ "$1" = slow ] && sleep 2
printf '{"ran":"%s"}\n' "$1" >>"$HOOK_LOG"
printf '{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"denied by %s"}}' "$1"
exit 0
