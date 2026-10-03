#!/bin/sh
# Logs its payload, then prints a marker on stdout (as plain text and as JSON)
# and another on stderr: what a sessionEnd hook says, to be looked for in the
# transcript.
IN=$(cat)
printf '%s\n' "$IN" >>"$HOOK_LOG"
echo "SESSION-END-STDOUT-MARKER"
echo '{"user_message":"SESSION-END-JSON-MARKER","agent_message":"SESSION-END-AGENT-MARKER","additional_context":"SESSION-END-CONTEXT-MARKER"}'
echo "SESSION-END-STDERR-MARKER" >&2
exit 0
