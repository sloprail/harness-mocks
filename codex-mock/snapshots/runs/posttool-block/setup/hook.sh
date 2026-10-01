#!/bin/sh
# PostToolUse gives feedback by exit 2 (stderr) for POSTBLOCK, and by JSON
# decision block for nothing else; every payload is logged.
IN=$(cat)
printf '%s\n' "$IN" >>"$HOOK_LOG"
ev=$(printf '%s' "$IN" | jq -r '.hook_event_name')
cmd=$(printf '%s' "$IN" | jq -r '.tool_input.command // ""')
if [ "$ev" = PostToolUse ]; then case "$cmd" in *POSTBLOCK*) echo "POST-FEEDBACK-MSG" >&2; exit 2 ;; esac; fi
exit 0
