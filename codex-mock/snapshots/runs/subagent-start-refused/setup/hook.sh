#!/bin/sh
# Every event logs its payload. The sub-agent's start hook tries to refuse it:
# exit 2 with a reason on stderr.
IN=$(cat)
printf '%s\n' "$IN" >>"$HOOK_LOG"
case $(printf '%s' "$IN" | jq -r '.hook_event_name') in
  SubagentStart) echo "NO-SUBAGENT-REASON" >&2; exit 2 ;;
esac
exit 0
