#!/bin/sh
# A command hook beside an HTTP hook whose endpoint is not listening: it logs
# its payload, so a run shows the call went ahead although the HTTP hook failed.
IN=$(cat)
printf '%s\n' "$IN" >>"$HOOK_LOG"
exit 0
