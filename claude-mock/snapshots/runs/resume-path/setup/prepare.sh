#!/bin/sh
# The earlier session the run resumes by the path of its transcript file: a real
# one, made here so that the run itself is one claude invocation.
HOOK_LOG=/dev/null claude -p --model haiku --dangerously-skip-permissions --session-id 00000000-0000-4000-8000-0000000000e1 'Reply only ONE' </dev/null >/dev/null
