#!/bin/sh
# Every event logs its payload. SubagentStop blocks every time by JSON, up to 15
# times (so a run cannot go on forever), to see whether Codex overrides a block.
IN=$(cat)
printf '%s\n' "$IN" >>"$HOOK_LOG"
EV=$(printf '%s' "$IN" | jq -r '.hook_event_name')
case "$EV" in
  SubagentStop)
    N=$(cat "$TMPDIR/substopn" 2>/dev/null || echo 0); N=$((N + 1)); echo $N >"$TMPDIR/substopn"
    if [ $N -le 15 ]; then echo "{\"decision\":\"block\",\"reason\":\"SUBSTOP-BLOCK-$N\"}"; fi ;;
esac
exit 0
