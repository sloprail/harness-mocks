#!/bin/sh
# Every event logs its payload.
IN=$(cat)
printf '%s\n' "$IN" >>"$HOOK_LOG"
exit 0
