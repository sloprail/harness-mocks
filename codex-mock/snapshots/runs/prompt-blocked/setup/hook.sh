#!/bin/sh
# UserPromptSubmit blocks the prompt by exit 2 with a reason on stderr.
IN=$(cat)
printf '%s\n' "$IN" >>"$HOOK_LOG"
ev=$(printf '%s' "$IN" | jq -r '.hook_event_name')
if [ "$ev" = UserPromptSubmit ]; then echo "PROMPT-BLOCK-MSG" >&2; exit 2; fi
exit 0
