#!/bin/sh
# Every event logs its payload with whether its transcript file exists yet.
IN=$(cat)
P=$(printf '%s' "$IN" | jq -r '.transcript_path')
if [ -f "$P" ]; then E=true; else E=false; fi
printf '%s' "$IN" | jq -c --argjson e "$E" '. + {transcript_exists: $e}' >>"$HOOK_LOG"
