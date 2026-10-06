#!/bin/sh
# The Interrupt hooks log their payload, wait, and log that they finished: the default timeout
# (none configured), one of 3 seconds over a 2-second wait, one of 10 seconds over a 5-second wait.
IN=$(cat)
printf '%s\n{"ran":"%s"}\n' "$IN" "$1" >>"$HOOK_LOG"
sleep "${1#* }"
printf '{"done":"%s"}\n' "${1% *}" >>"$HOOK_LOG"
exit 0
