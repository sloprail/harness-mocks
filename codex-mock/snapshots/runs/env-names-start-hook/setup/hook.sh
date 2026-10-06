#!/bin/sh
# Every event logs its payload, then the NAMES (never the values) of the variables the hook process itself
# received whose name could name an env file: it holds ENV, FILE or SOURCE.
IN=$(cat)
printf '%s\n' "$IN" >>"$HOOK_LOG"
printf '%s\n' "$(env | cut -d= -f1 | grep -i -E 'ENV|FILE|SOURCE' | sort | tr '\n' ' ')" | jq -R -c '{hook_env_names_like_an_env_file: split(" ") | map(select(. != ""))}' >>"$HOOK_LOG"
exit 0
