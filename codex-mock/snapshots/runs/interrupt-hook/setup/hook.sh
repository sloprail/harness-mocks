#!/bin/sh
# Every event logs its payload.
cat >>"$HOOK_LOG"
echo >>"$HOOK_LOG"
exit 0
