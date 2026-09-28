#!/bin/sh
IN=$(cat); D="$(dirname "$0")"; printf '%s\n' "$IN" >> "$D/payloads.jsonl"
EV=$(printf '%s' "$IN" | sed -n 's/.*"hook_event_name":"\([A-Za-z]*\)".*/\1/p')
case "$EV" in
 SessionStart) echo '{"hookSpecificOutput":{"hookEventName":"SessionStart","additionalContext":"SS-CTX"}}';;
 PostToolUse) echo '{"hookSpecificOutput":{"hookEventName":"PostToolUse","additionalContext":"POST-CTX"}}';;
 SubagentStart) echo "SAS-STDERR" >&2;;
 SubagentStop) M=$(cat "$D/sstopn" 2>/dev/null || echo 0); M=$((M+1)); echo $M > "$D/sstopn"; if [ $M = 1 ]; then echo "SUB-BLOCK" >&2; exit 2; fi; echo "SAST-STDERR" >&2;;
esac
exit 0
