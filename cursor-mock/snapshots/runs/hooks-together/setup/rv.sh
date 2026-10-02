#!/bin/sh
# Two hooks of one event that wait for each other: each one marks that it has
# started, then waits (up to 5 s) for the other's mark. Each logs whether it saw
# the other's, which it can only have done if the other had started while it
# was still running: the two ran at the same time.
#   rv.sh <me> <other>
cat >/dev/null
touch "$HOOK_LOG.$1"
i=0
while [ ! -f "$HOOK_LOG.$2" ] && [ "$i" -lt 50 ]; do sleep 0.1; i=$((i + 1)); done
if [ -f "$HOOK_LOG.$2" ]; then saw="saw-$2"; else saw="missed-$2"; fi
printf '{"hook_result":{"script":"rv-%s","event":"beforeShellExecution","command":"%s"}}\n' "$1" "$saw" >>"$HOOK_LOG"
exit 0
