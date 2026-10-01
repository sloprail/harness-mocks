#!/bin/sh
# Exit statuses combined with stdout: JSON that decides on exit 1, malformed or
# schema-invalid JSON, stderr on exit 0, and exit 1 on events where only 2
# blocks. Logs each payload and the exit code it chose.
IN=$(cat)
printf '%s\n' "$IN" >>"$HOOK_LOG"
ev=$(printf '%s' "$IN" | jq -r '.hook_event_name')
cmd=$(printf '%s' "$IN" | jq -r '.tool_input.command // ""')
code=0
case "$ev" in
  UserPromptSubmit) echo "prompt hook failed with exit 1" >&2; code=1 ;;
  PreToolUse) case "$cmd" in
      *"echo a"*) echo "quiet stderr on exit 0" >&2 ;;
      *"echo b"*) printf '%s' '{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"denied by JSON on exit 1"}}'; code=1 ;;
      *"echo c"*) printf '%s' '{not json at all}' ;;
      *"echo d"*) printf '%s' '{"decision": 42}'; echo "schema-invalid on exit 1" >&2; code=1 ;;
      *"echo e"*) printf '%s' '{"unclosed": 1' ;;
      *"echo f"*) printf '%s\n%s' '[1, 2]' '"just a string"' ;;
      *"echo g"*) printf '%s' '{"decision": 42}' ;;
      *"echo i"*) printf '%s\n%s' '{"x": 1}' '{"y": 2}' ;;
      *"echo j"*) printf '%s\n%s' '{"x": 1}' '{"decision": "block", "reason": "lines with a field"}' ;;
      *"echo k"*) printf '%s' '{not json on exit 1}'; echo "malformed on exit 1" >&2; code=1 ;;
      *"echo h"*) printf '%s' '{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"the JSON reason wins"}}'; echo "the stderr loses" >&2; code=2 ;;
    esac ;;
  Stop) echo "stop hook failed with exit 1" >&2; code=1 ;;
esac
printf '{"hook_result":{"event":"%s","command":"%s","exit":%s}}\n' "$ev" "$cmd" "$code" >>"$HOOK_LOG"
exit $code
