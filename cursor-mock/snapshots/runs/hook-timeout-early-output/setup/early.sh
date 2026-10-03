#!/bin/sh
# Prints its deny at once, then outlives its 1 s timeout: the deny is output
# the hook gave before the timeout expired.
cat >/dev/null
echo "{\"hook_ran\":\"$1\",\"phase\":\"started\"}" >>"$HOOK_LOG"
echo '{"permission":"deny","user_message":"EARLY-DENY","agent_message":"EARLY-DENY"}'
sleep 5
echo "{\"hook_ran\":\"$1\",\"phase\":\"finished\"}" >>"$HOOK_LOG"
exit 0
