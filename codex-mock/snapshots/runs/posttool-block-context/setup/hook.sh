#!/bin/sh
# PostToolUse answers JSON for POSTBLOCK: a block decision with a reason, and added
# context together; every payload is logged.
IN=$(cat)
printf '%s\n' "$IN" >>"$HOOK_LOG"
ev=$(printf '%s' "$IN" | jq -r '.hook_event_name')
cmd=$(printf '%s' "$IN" | jq -r '.tool_input.command // ""')
if [ "$ev" = PostToolUse ]; then case "$cmd" in *POSTBLOCK*) echo '{"decision":"block","reason":"POST-REASON-MSG","hookSpecificOutput":{"hookEventName":"PostToolUse","additionalContext":"POST-CONTEXT-MSG"}}'; exit 0 ;; esac; fi
exit 0
