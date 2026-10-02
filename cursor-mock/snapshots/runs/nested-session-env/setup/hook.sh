#!/bin/sh
# Every event logs its payload, then every CURSOR* and CLAUDE* variable the
# hook process itself received (hooks are child processes too): all of them,
# not a list we chose. The account's email is the one value never logged.
IN=$(cat)
printf '%s\n' "$IN" >>"$HOOK_LOG"
jq -cn '{hook_env: (env | with_entries(select((.key | test("^(CURSOR|CLAUDE)")) and .key != "CURSOR_USER_EMAIL")))}' >>"$HOOK_LOG"
exit 0
