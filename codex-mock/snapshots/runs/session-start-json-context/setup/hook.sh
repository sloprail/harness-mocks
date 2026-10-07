#!/bin/sh
# SessionStart prints only the hookSpecificOutput JSON with additionalContext; the payload is logged.
cat >>"$HOOK_LOG"
printf '%s' '{"hookSpecificOutput":{"hookEventName":"SessionStart","additionalContext":"CTX-JSON-2"}}'
exit 0
