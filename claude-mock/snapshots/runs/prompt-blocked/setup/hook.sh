#!/bin/sh
# UserPromptSubmit exits 2: the prompt is blocked. Logs the payload.
IN=$(cat)
printf '%s\n' "$IN" >>"$HOOK_LOG"
echo "prompt refused by the hook" >&2
exit 2
