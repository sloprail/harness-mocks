#!/bin/sh
# Every event logs its payload and adds context naming it.
IN=$(cat)
printf '%s\n' "$IN" >>"$HOOK_LOG"
E=$(printf '%s' "$IN" | jq -r .hook_event_name)
printf '{"hookSpecificOutput":{"hookEventName":"%s","additionalContext":"CTX-%s"}}\n' "$E" "$E"
