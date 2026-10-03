#!/bin/sh
# UserPromptSubmit adds context as JSON hookSpecificOutput.additionalContext; every payload is logged.
IN=$(cat)
printf '%s\n' "$IN" >>"$HOOK_LOG"
ev=$(printf '%s' "$IN" | jq -r '.hook_event_name')
if [ "$ev" = UserPromptSubmit ]; then echo '{"hookSpecificOutput":{"hookEventName":"UserPromptSubmit","additionalContext":"The secret word is BANANA."}}'; fi
exit 0
