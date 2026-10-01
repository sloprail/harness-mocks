#!/bin/sh
# UserPromptSubmit exits 2 and asks Claude Code to leave the prompt out of the
# block message. Logs the payload.
IN=$(cat)
printf '%s\n' "$IN" >>"$HOOK_LOG"
printf '%s' '{"hookSpecificOutput":{"hookEventName":"UserPromptSubmit","suppressOriginalPrompt":true}}'
echo "prompt refused by the hook" >&2
exit 2
