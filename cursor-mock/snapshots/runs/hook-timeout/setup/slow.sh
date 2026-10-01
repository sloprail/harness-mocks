#!/bin/sh
# Runs longer than its 1 s timeout, with a background child that would log
# after 3 s, and prints a deny it would give if it got to finish.
cat >/dev/null
echo "{\"hook_ran\":\"$1\",\"phase\":\"started\"}" >>"$HOOK_LOG"
( sleep 3; echo "{\"hook_ran\":\"$1\",\"phase\":\"grandchild-finished\"}" >>"$HOOK_LOG" ) &
sleep 5
echo '{"permission":"deny","user_message":"SLOW-DENY"}'
echo "{\"hook_ran\":\"$1\",\"phase\":\"finished\"}" >>"$HOOK_LOG"
exit 0
