#!/bin/sh
# Every event logs its payload and exits 0, so the log shows which events fire.
IN=$(cat)
printf '%s\n' "$IN" >>"$HOOK_LOG"
