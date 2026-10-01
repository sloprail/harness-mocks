#!/bin/sh
# Every event logs its payload, then every CODEX* variable the hook process
# itself received: the second half of what subprocess-session-env is about
# (hooks are child processes too). All of them, not a list we chose: a run
# records only what it logs.
IN=$(cat)
printf '%s\n' "$IN" >>"$HOOK_LOG"
jq -cn '{hook_env: (env | with_entries(select(.key | startswith("CODEX"))))}' >>"$HOOK_LOG"
exit 0
