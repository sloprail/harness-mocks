#!/usr/bin/env bash
# A mock of the classify judge that decides from its input: the prompt names each changed file's diff
# (diff="..."); stamping a record with time.Now() directly is a silent choice (the time source), anything else is local.
IFS= read -r -d '' input
diffs="$(printf '%s\n' "$input" | sed -n 's/.*diff="\([^"]*\)".*/\1/p')"
hit=""
for d in $diffs; do grep -q 'time[.]Now()' "$d" && hit=1; done
if [ -n "$hit" ]; then
  echo '{"pass":false,"reasoning":"No ADR decides this: the time source is chosen in passing. Ask the user to decide it."}'
else
  echo '{"pass":true,"reasoning":"every choice is local"}'
fi
