#!/bin/sh
# beforeSubmitPrompt refuses the prompt: it answers continue:false with a user
# message, and exits 2 with a reason on stderr. The payload is logged.
IN=$(cat)
printf '%s\n' "$IN" >>"$HOOK_LOG"
EV=$(printf '%s' "$IN" | jq -r '.hook_event_name')
printf '{"hook_result":{"event":"%s","fired":true}}\n' "$EV" >>"$HOOK_LOG"
echo "PROMPT-BLOCK-MSG" >&2
printf '{"continue":false,"user_message":"PROMPT-BLOCK-MSG"}\n'
exit 2
