#!/bin/sh
# SessionStart prints a plain line, then the hookSpecificOutput JSON with additionalContext; the payload is logged.
cat >>"$HOOK_LOG"
echo CTX-PLAIN-3
printf '%s' '{"hookSpecificOutput":{"hookEventName":"SessionStart","additionalContext":"CTX-JSON-4"}}'
exit 0
