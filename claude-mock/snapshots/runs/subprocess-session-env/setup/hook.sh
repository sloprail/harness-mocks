#!/bin/sh
# Every event logs its payload, then the environment the hook process itself
# received: the second half of what subprocess-session-env is about (hooks are
# child processes too).
IN=$(cat)
printf '%s\n' "$IN" >>"$HOOK_LOG"
printf '{"hook_env":{"CLAUDE_CODE_SESSION_ID":"%s","CLAUDECODE":"%s","CLAUDE_CODE_ENTRYPOINT":"%s"}}\n' \
  "${CLAUDE_CODE_SESSION_ID-<unset>}" "${CLAUDECODE-<unset>}" "${CLAUDE_CODE_ENTRYPOINT-<unset>}" >>"$HOOK_LOG"
exit 0
