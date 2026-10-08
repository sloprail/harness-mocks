#!/bin/sh
# Logs that it ran: one line for each time the harness runs it.
cat >/dev/null
printf '{"hook_ran":"%s"}\n' "$1" >>"$HOOK_LOG"
exit 0
