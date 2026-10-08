#!/bin/sh
# SessionStart logs its payload and adds context naming its source.
IN=$(cat)
printf '%s\n' "$IN" >>"$HOOK_LOG"
S=$(printf '%s' "$IN" | jq -r .source)
printf '{"hookSpecificOutput":{"hookEventName":"SessionStart","additionalContext":"CTX-%s"}}\n' "$S"
