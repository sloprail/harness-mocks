#!/bin/sh
# Every event logs its payload. PreToolUse refuses DENYME by JSON and EXIT2ME
# by exit 2; Stop blocks by JSON, then by exit 2, then lets the turn end.
IN=$(cat)
printf '%s\n' "$IN" >>"$HOOK_LOG"
EV=$(printf '%s' "$IN" | jq -r '.hook_event_name')
case "$EV" in
  SessionStart) echo '{"hookSpecificOutput":{"hookEventName":"SessionStart","additionalContext":"SS-CTX"}}' ;;
  PreToolUse)
    if printf '%s' "$IN" | grep -q DENYME; then echo '{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"DENY-REASON"}}'; fi
    if printf '%s' "$IN" | grep -q EXIT2ME; then echo "PRE-BLOCK-MSG" >&2; exit 2; fi ;;
  Stop)
    N=$(cat "$TMPDIR/stopn" 2>/dev/null || echo 0); N=$((N + 1)); echo $N >"$TMPDIR/stopn"
    if [ $N = 1 ]; then echo '{"decision":"block","reason":"BLOCK-JSON-REASON"}'; exit 0; fi
    if [ $N = 2 ]; then echo "BLOCK-EXIT2-REASON" >&2; exit 2; fi ;;
esac
exit 0
