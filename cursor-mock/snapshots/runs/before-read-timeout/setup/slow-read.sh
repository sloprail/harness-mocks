#!/bin/sh
# A beforeReadFile hook that runs longer than its 1 s timeout on the file named
# by its argument (and allows any other read at once). It logs that it started,
# what a background child logs after 3 s, and that it finished, and prints the
# deny it would give if it got to finish.
FILE=$(cat | jq -r '.file_path // ""')
if [ "$(basename "$FILE")" != "$1" ]; then echo '{"permission":"allow"}'; exit 0; fi
echo "{\"hook_ran\":\"$1\",\"phase\":\"started\"}" >>"$HOOK_LOG"
( sleep 3; echo "{\"hook_ran\":\"$1\",\"phase\":\"grandchild-finished\"}" >>"$HOOK_LOG" ) &
sleep 5
echo '{"permission":"deny","user_message":"SLOW-DENY"}'
echo "{\"hook_ran\":\"$1\",\"phase\":\"finished\"}" >>"$HOOK_LOG"
exit 0
