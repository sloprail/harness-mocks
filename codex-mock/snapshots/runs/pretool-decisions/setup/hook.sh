#!/bin/sh
# Three PreToolUse hooks decide each call at once, by the argument they run
# with: allow, deny and ask. A hook decides only the commands that name it
# (ALLOW, DENY, ASK in the command); every payload is logged.
IN=$(cat)
printf '%s\n' "$IN" >>"$HOOK_LOG"
cmd=$(printf '%s' "$IN" | jq -r '.tool_input.command // ""')
decide() { printf '{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"%s","permissionDecisionReason":"%s by hook %s"}}' "$1" "$1" "$2"; }
case "$1:$cmd" in
  allow:*ALLOW*) decide allow allow ;;
  deny:*DENY*) decide deny deny ;;
  ask:*ASK*) decide ask ask ;;
esac
exit 0
