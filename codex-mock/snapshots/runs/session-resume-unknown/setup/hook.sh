#!/bin/sh
printf '%s\n' "$(cat)" >>"$HOOK_LOG"
exit 0
