#!/bin/sh
# The earlier session the run continues: a real one, made here so that the run
# itself is one claude invocation (--continue takes the most recent session of
# the directory).
HOOK_LOG=/dev/null claude -p --model haiku --dangerously-skip-permissions --session-id 00000000-0000-4000-8000-0000000000d1 'Reply only ONE' </dev/null >/dev/null
