#!/bin/sh
# Every hook logs its payload and its kind; the Interrupt ones answer: JSON with a systemMessage,
# plain text, and exit 2 with a reason on stderr.
IN=$(cat)
printf '%s\n{"ran":"%s"}\n' "$IN" "$1" >>"$HOOK_LOG" # one write: the hooks run at once
case "$1" in
  json) echo '{"systemMessage":"INT-SYSMSG","continue":true}' ;;
  plain) echo INT-PLAIN ;;
  exit2) echo INT-EXIT2-REASON >&2; exit 2 ;;
esac
exit 0
