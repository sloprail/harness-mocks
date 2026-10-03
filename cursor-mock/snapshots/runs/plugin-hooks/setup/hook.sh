#!/bin/sh
# Logs which hook ran: the project's own, or a plugin's (the argument names it).
cat >/dev/null
printf '{"hook_ran":"%s"}\n' "$1" >>"$HOOK_LOG"
exit 0
