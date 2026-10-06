#!/bin/sh
# SubagentStart answers with a systemMessage; every event logs its payload.
IN=$(cat)
printf '%s\n' "$IN" >>"$HOOK_LOG"
case "$IN" in *'"SubagentStart"'*) echo '{"systemMessage":"SA-SYSMSG"}' ;; esac
exit 0
