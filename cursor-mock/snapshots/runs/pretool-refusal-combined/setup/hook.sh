#!/bin/sh
# Logs each payload it is given (the failure hook's, with the refusal's message).
IN=$(cat)
printf '%s\n' "$IN" >>"$HOOK_LOG"
exit 0
