#!/bin/sh
# Every event logs its payload and succeeds.
printf '%s\n' "$(cat)" >>"$HOOK_LOG"
exit 0
