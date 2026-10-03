#!/bin/sh
# Every event logs its payload, then whether the transcript file it names
# exists right now.
IN=$(cat)
printf '%s\n' "$IN" >>"$HOOK_LOG"
EV=$(printf '%s' "$IN" | jq -r '.hook_event_name')
P=$(printf '%s' "$IN" | jq -r '.transcript_path')
if [ -e "$P" ]; then E=true; else E=false; fi
printf '{"probe":"%s","transcript_exists":%s}\n' "$EV" "$E" >>"$HOOK_LOG"
exit 0
