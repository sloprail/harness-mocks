#!/bin/sh
IN=$(cat); D="$(dirname "$0")"; printf '%s\n' "$IN" >> "$D/payloads.jsonl"
EV=$(printf '%s' "$IN" | sed -n 's/.*"hook_event_name":"\([A-Za-z]*\)".*/\1/p')
case "$EV" in
 SessionStart) echo '{"hookSpecificOutput":{"hookEventName":"SessionStart","additionalContext":"SS-CTX"}}';;
 PreToolUse) if printf '%s' "$IN" | grep -q DENYME; then echo '{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"DENY-REASON"}}'; fi
             if printf '%s' "$IN" | grep -q EXIT2ME; then echo "PRE-BLOCK-MSG" >&2; exit 2; fi;;
 Stop) N=$(cat "$D/stopn" 2>/dev/null || echo 0); N=$((N+1)); echo $N > "$D/stopn"
   if [ $N = 1 ]; then echo '{"decision":"block","reason":"BLOCK-JSON-REASON"}'; exit 0; fi
   if [ $N = 2 ]; then echo "BLOCK-EXIT2-REASON" >&2; exit 2; fi;;
esac
exit 0
