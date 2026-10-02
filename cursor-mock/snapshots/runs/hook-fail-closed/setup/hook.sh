#!/bin/sh
# One hook for every event; each call exits with the status its command asks
# for, and the payload and that status are logged.
#   EXIT2ME  beforeShellExecution exits 2 (stderr says why)
#   EXIT1ME  beforeShellExecution exits 1; EXIT3ME exits 3
#   BADJSON  beforeShellExecution prints text that is not JSON and exits 0
#   AFTER2ME afterShellExecution exits 2
IN=$(cat)
printf '%s\n' "$IN" >>"$HOOK_LOG"
EV=$(printf '%s' "$IN" | jq -r '.hook_event_name')
code=0
case "$EV" in
  beforeShellExecution)
    if printf '%s' "$IN" | grep -q EXIT2ME; then echo "SHELL-BLOCK-MSG" >&2; code=2; fi
    if printf '%s' "$IN" | grep -q EXIT1ME; then echo "SHELL-EXIT1-MSG" >&2; code=1; fi
    if printf '%s' "$IN" | grep -q EXIT3ME; then echo "SHELL-EXIT3-MSG" >&2; code=3; fi
    if printf '%s' "$IN" | grep -q BADJSON; then echo "this is not json"; fi ;;
  afterShellExecution)
    if printf '%s' "$IN" | grep -q AFTER2ME; then echo "AFTER-EXIT2-MSG" >&2; code=2; fi ;;
esac
printf '{"hook_result":{"event":"%s","exit":%s}}\n' "$EV" "$code" >>"$HOOK_LOG"
exit $code
