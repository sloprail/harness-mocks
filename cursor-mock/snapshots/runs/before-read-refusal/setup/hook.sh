#!/bin/sh
# Every event logs its payload and the exit status this script gave it.
# sessionStart makes the four files the prompt names (a hook runs from the
# project root). beforeReadFile answers each read differently: a.txt by exit 2,
# b.txt by a JSON deny, c.txt by exit 0 with output that is not JSON, d.txt by
# exit 1 (a crash, not a block), f.txt by a JSON response with a permission that
# is not one ("maybe"). closed.sh, a failClosed hook, allows by JSON and crashes
# (exit 1) on e.txt.
IN=$(cat)
printf '%s\n' "$IN" >>"$HOOK_LOG"
EV=$(printf '%s' "$IN" | jq -r '.hook_event_name')
FILE=$(printf '%s' "$IN" | jq -r '.file_path // ""')
code=0
case "$EV" in
  sessionStart)
    for f in a b c d e f; do echo "CONTENT-$f" >"$f.txt"; done ;;
  beforeReadFile)
    case "$(basename "$FILE")" in
      a.txt) echo "EXIT2-MSG" >&2; code=2 ;;
      b.txt) echo '{"permission":"deny","user_message":"JSON-DENY-MSG","agent_message":"JSON-AGENT-MSG"}' ;;
      c.txt) echo BADJSON ;;
      d.txt) echo "CRASH-MSG" >&2; code=1 ;;
      f.txt) echo '{"permission":"maybe"}' ;;
    esac ;;
esac
printf '{"hook_result":{"event":"%s","exit":%s}}\n' "$EV" "$code" >>"$HOOK_LOG"
exit $code
