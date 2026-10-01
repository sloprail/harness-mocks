#!/bin/sh
# One hook for every event; each event exits with the code the scenario
# exercises, and logs its payload and that code.
IN=$(cat)
printf '%s\n' "$IN" >>"$HOOK_LOG"
ev=$(printf '%s' "$IN" | jq -r '.hook_event_name')
tool=$(printf '%s' "$IN" | jq -r '.tool_name // ""')
code=0
case "$ev" in
  SessionStart) echo "session-start stderr on exit 2" >&2; code=2 ;;
  UserPromptSubmit) echo "The secret word is BANANA." ;;
  PreToolUse) [ "$tool" = Bash ] && { echo "Blocked: Bash is off in this scenario" >&2; code=2; } ;;
  PostToolUse) echo "post-tool warning on exit 1" >&2; code=1 ;;
  Stop) if [ ! -f "$TMPDIR/stop-blocked-once" ]; then : >"$TMPDIR/stop-blocked-once"; echo "Before finishing, reply with the single word STOPPED-ONCE." >&2; code=2; fi ;;
  SessionEnd) echo "session-end stderr on exit 1" >&2; code=1 ;;
esac
printf '{"hook_result":{"event":"%s","tool":"%s","exit":%s}}\n' "$ev" "$tool" "$code" >>"$HOOK_LOG"
exit $code
