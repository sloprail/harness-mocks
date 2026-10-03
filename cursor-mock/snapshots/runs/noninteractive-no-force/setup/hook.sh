#!/bin/sh
# Every event logs its payload; sessionEnd also logs what note.txt holds by then
# (a hook runs from the project root), and whether it exists at all.
IN=$(cat)
printf '%s\n' "$IN" >>"$HOOK_LOG"
EV=$(printf '%s' "$IN" | jq -r '.hook_event_name')
if [ "$EV" = sessionEnd ]; then
  if [ -e note.txt ]; then
    printf '{"hook_result":{"event":"files","note_exists":true,"note":"%s"}}\n' "$(cat note.txt)" >>"$HOOK_LOG"
  else
    printf '{"hook_result":{"event":"files","note_exists":false}}\n' >>"$HOOK_LOG"
  fi
fi
exit 0
