#!/bin/sh
# One SubagentStop hook: on its first two calls it exits 2 with a reason on stderr and also prints
# continue:false; on later calls it lets the sub-agent stop. Each call logs its payload.
IN=$(cat)
printf '%s\n' "$IN" >>"$HOOK_LOG"
N=$(cat "$TMPDIR/substopn" 2>/dev/null || echo 0); N=$((N + 1)); echo $N >"$TMPDIR/substopn"
printf '{"hook_result":{"call":%s}}\n' "$N" >>"$HOOK_LOG"
if [ $N -le 2 ]; then
  echo '{"continue":false}'
  echo "EXIT2-REASON" >&2
  exit 2
fi
exit 0
