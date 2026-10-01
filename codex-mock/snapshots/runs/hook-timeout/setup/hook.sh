#!/bin/sh
# slow: a PreToolUse hook that outlives its 1 second timeout, leaves a
# grandchild running and has printed a deny that must be discarded.
# probe: after the tool call, whether that grandchild is still alive.
IN=$(cat)
case "$1" in
  slow)
    sleep 30 &
    echo $! >"$TMPDIR/grandchild"
    echo '{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"TIMEOUT-DENY"}}'
    wait ;;
  probe)
    if kill -0 "$(cat "$TMPDIR/grandchild")" 2>/dev/null; then s=alive; else s=gone; fi
    printf '{"grandchild":"%s"}\n' "$s" >>"$HOOK_LOG" ;;
esac
exit 0
