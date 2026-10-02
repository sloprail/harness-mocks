#!/bin/sh
# Every event logs its payload and the exit status this script gave it.
# preToolUse refuses DENYME and DENYWINS by JSON and EXIT2PRE by exit 2;
# beforeShellExecution refuses DENYSHELL by JSON and EXIT2SHELL by exit 2.
# (A second preToolUse hook, allow.sh, allows everything: DENYWINS is the call
# both decide.)
IN=$(cat)
printf '%s\n' "$IN" >>"$HOOK_LOG"
EV=$(printf '%s' "$IN" | jq -r '.hook_event_name')
code=0
case "$EV" in
  preToolUse)
    if printf '%s' "$IN" | grep -qE 'DENYME|DENYWINS'; then echo '{"permission":"deny","user_message":"USER-DENY-MSG","agent_message":"AGENT-DENY-MSG"}'; fi
    if printf '%s' "$IN" | grep -q EXIT2PRE; then echo "PRE-BLOCK-MSG" >&2; code=2; fi ;;
  beforeShellExecution)
    if printf '%s' "$IN" | grep -q DENYSHELL; then echo '{"permission":"deny","user_message":"SHELL-USER-DENY-MSG","agent_message":"SHELL-AGENT-DENY-MSG"}'; fi
    if printf '%s' "$IN" | grep -q EXIT2SHELL; then echo "SHELL-BLOCK-MSG" >&2; code=2; fi ;;
esac
printf '{"hook_result":{"event":"%s","exit":%s}}\n' "$EV" "$code" >>"$HOOK_LOG"
exit $code
