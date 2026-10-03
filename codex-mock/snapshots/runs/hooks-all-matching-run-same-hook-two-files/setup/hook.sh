#!/bin/sh
# Logs that it ran, with the name it was given: the same hook is configured in
# the user layer and in the project layer, and a different one only in the
# project layer.
cat >/dev/null
printf '{"ran":"%s"}\n' "$1" >>"$HOOK_LOG"
exit 0
