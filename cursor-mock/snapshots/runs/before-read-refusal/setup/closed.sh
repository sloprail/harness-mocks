#!/bin/sh
# A failClosed beforeReadFile hook: it allows by JSON, and crashes on e.txt.
IN=$(cat)
FILE=$(printf '%s' "$IN" | jq -r '.file_path // ""')
code=0
if [ "$(basename "$FILE")" = e.txt ]; then echo "CLOSED-CRASH" >&2; code=1; else echo '{"permission":"allow"}'; fi
printf '{"hook_result":{"event":"closed","exit":%s}}\n' "$code" >>"$HOOK_LOG"
exit $code
