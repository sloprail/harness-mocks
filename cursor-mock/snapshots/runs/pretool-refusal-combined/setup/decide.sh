#!/bin/sh
# A preToolUse hook: .cursor/hooks/decide.sh <name> <how> <command word>. When
# the call's command holds the word, it refuses it the way <how> says: "deny"
# prints a JSON deny with the user_message "<name>-DENY", "block" writes
# "<name>-BLOCK" on stderr and exits 2. It logs that it ran either way.
IN=$(cat)
CMD=$(printf '%s' "$IN" | jq -r '.tool_input.command // ""')
printf '{"hook_ran":"%s","event":"preToolUse","command":"%s"}\n' "$1" "$CMD" >>"$HOOK_LOG"
if printf '%s' "$CMD" | grep -qE "$3"; then
  case "$2" in
    deny) echo "{\"permission\":\"deny\",\"user_message\":\"$1-DENY\"}" ;;
    block) echo "$1-BLOCK" >&2; exit 2 ;;
  esac
fi
exit 0
