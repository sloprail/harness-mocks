#!/bin/sh
# Every event logs its raw payload. The conversation records of the transcript are kept at
# preCompact (the turn_ended marker the harness writes at the end of a turn and moves behind what
# follows is not one); the last event then logs, after its payload, whether the transcript
# still begins with those bytes (hook_result: prefix_kept) and whether it is the same file; it is
# asked once, at sessionEnd, where nothing else is fired beside it.
# afterAgentResponse and stop of one turn are fired side by side, so the stop script waits (at
# most 10 s) for its turn's afterAgentResponse to be logged: every recording has them in the
# order the turn happens.
IN=$(cat)
EV=$(printf '%s' "$IN" | jq -r '.hook_event_name')
if [ "$EV" = stop ]; then
  n=0
  while [ "$(grep -c '"hook_event_name":"afterAgentResponse"' "$HOOK_LOG" 2>/dev/null)" -le "$(grep -c '"hook_event_name":"stop"' "$HOOK_LOG" 2>/dev/null)" ] && [ "$n" -lt 200 ]; do
    n=$((n + 1)); sleep 0.05
  done
fi
printf '%s\n' "$IN" >>"$HOOK_LOG"
T=$(printf '%s' "$IN" | jq -r '.transcript_path // empty')
if [ "$EV" = preCompact ] && [ -n "$T" ]; then
  grep -v '"type":"turn_ended"' "$T" >"$TMPDIR/pre.jsonl"
  echo "$T" >"$TMPDIR/pre.path"
elif [ "$EV" = sessionEnd ] && [ -f "$TMPDIR/pre.jsonl" ] && [ -n "$T" ]; then
  K=false
  S=false
  [ "$(head -c "$(wc -c <"$TMPDIR/pre.jsonl")" "$T" | cksum)" = "$(cksum <"$TMPDIR/pre.jsonl")" ] && K=true
  [ "$T" = "$(cat "$TMPDIR/pre.path")" ] && S=true
  printf '{"hook_result":{"event":"%s","prefix_kept":%s,"same_file":%s}}\n' "$EV" "$K" "$S" >>"$HOOK_LOG"
fi
exit 0
