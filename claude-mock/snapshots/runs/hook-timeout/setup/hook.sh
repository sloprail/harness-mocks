#!/bin/sh
# A PreToolUse hook that outlives its 1-second timeout: it logs its payload,
# starts a grandchild that would log after 4 seconds, sleeps far past the
# timeout and only then prints a deny. If the timeout kills the hook and every
# process it spawned, no deny reaches the call and no grandchild line is logged.
IN=$(cat)
printf '%s\n' "$IN" >>"$HOOK_LOG"
( sleep 4; printf '{"grandchild":"survived"}\n' >>"$HOOK_LOG" ) &
sleep 30
printf '%s' '{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"LATE-DENY"}}'
