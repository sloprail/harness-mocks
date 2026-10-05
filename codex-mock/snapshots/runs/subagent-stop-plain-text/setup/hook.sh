#!/bin/sh
# SubagentStop answers with plain text on stdout and exit 0 (the docs: invalid for this event);
# every event logs its payload.
IN=$(cat)
printf '%s\n' "$IN" >>"$HOOK_LOG"
EV=$(printf '%s' "$IN" | jq -r '.hook_event_name')
case "$EV" in
  SubagentStop) echo "SUBSTOP-PLAIN-TEXT" ;;
esac
exit 0
