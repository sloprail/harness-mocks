#!/bin/sh
# Every event logs its payload; a PreToolUse of request_user_input answers it the way
# the hooks page documents for a local function tool: allow with updatedInput.
IN=$(cat)
printf '%s\n' "$IN" >>"$HOOK_LOG"
if [ "$(printf '%s' "$IN" | jq -r '.hook_event_name + "/" + .tool_name' 2>/dev/null)" = "PreToolUse/request_user_input" ]; then
  printf '%s' "$IN" | jq -c '{hookSpecificOutput: {hookEventName: "PreToolUse", permissionDecision: "allow", updatedInput: (.tool_input + {answers: {color: {answers: ["Blue"]}}})}}'
fi
exit 0
