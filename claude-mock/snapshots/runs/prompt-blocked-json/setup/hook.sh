#!/bin/sh
# UserPromptSubmit blocks by JSON (decision block with a reason), exiting 0.
# Logs the payload.
IN=$(cat)
printf "%s\n" "$IN" >>"$HOOK_LOG"
printf '%s' '{"decision":"block","reason":"BLOCKED-BY-JSON"}'
