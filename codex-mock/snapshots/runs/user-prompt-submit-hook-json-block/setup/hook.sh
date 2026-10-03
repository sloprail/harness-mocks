#!/bin/sh
# A UserPromptSubmit hook that refuses the prompt by JSON, with a reason.
IN=$(cat)
printf '%s\n' "$IN" >>"$HOOK_LOG"
echo '{"decision":"block","reason":"PROMPT-REFUSED-BY-JSON"}'
exit 0
