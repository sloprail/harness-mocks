#!/bin/sh
# Every event logs its payload. SessionStart and SubagentStart exit 2,
# UserPromptSubmit prints plain text, Stop exits 1, SessionEnd prints to both
# streams.
IN=$(cat)
printf '%s\n' "$IN" >>"$HOOK_LOG"
EV=$(printf '%s' "$IN" | jq -r '.hook_event_name')
case "$EV" in
  SessionStart) echo "SS-BLOCK-MSG" >&2; exit 2 ;;
  SubagentStart) echo "SAS-BLOCK-MSG" >&2; exit 2 ;;
  UserPromptSubmit) echo "UPS-PLAIN-OUT" ;;
  Stop) exit 1 ;;
  SessionEnd) echo "SE-OUT"; echo "SE-ERR" >&2 ;;
esac
exit 0
