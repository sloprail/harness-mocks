#!/bin/sh
# Every event logs its payload. sessionStart makes the three files the prompt
# names (a hook runs from the project root).
IN=$(cat)
printf '%s\n' "$IN" >>"$HOOK_LOG"
if [ "$(printf '%s' "$IN" | jq -r '.hook_event_name')" = sessionStart ]; then
  for f in a b c; do echo "CONTENT-$f" >"$f.txt"; done
fi
exit 0
