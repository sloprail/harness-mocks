#!/bin/sh
# The earlier session the run resumes by its name: a real one, made here with
# --name, so that the run itself is one claude invocation.
HOOK_LOG=/dev/null claude -p --model haiku --dangerously-skip-permissions --session-id 00000000-0000-4000-8000-0000000000f1 --name earlier-by-name 'Reply only ONE' </dev/null >/dev/null
