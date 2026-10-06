#!/bin/sh
# Every event logs its payload; the run has two workspace roots (the project
# and a second directory added with --add-dir).
cat >>"$HOOK_LOG"
echo >>"$HOOK_LOG"
exit 0
