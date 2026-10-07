#!/bin/sh
# SessionStart prints plain text; the payload is logged.
cat >>"$HOOK_LOG"
echo CTX-PLAIN-1
exit 0
