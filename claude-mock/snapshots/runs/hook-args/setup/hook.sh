#!/bin/sh
# Logs the arguments it was started with: how many, and each.
cat >/dev/null
printf '{"argc":%s,"argv1":"%s","argv2":"%s"}\n' "$#" "$1" "$2" >>"$HOOK_LOG"
exit 0
