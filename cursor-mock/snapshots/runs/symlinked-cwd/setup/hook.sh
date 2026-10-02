#!/bin/sh
# One hook for every event, run from a symlinked working directory. It logs each
# payload and, for each one: whether the file named by its transcript_path exists
# at that moment, how many session transcript files exist under the config
# directory at that moment (whatever the payload names), and what directory the
# hook itself ran in (logical and physical).
IN=$(cat)
printf '%s\n' "$IN" >>"$HOOK_LOG"
EV=$(printf '%s' "$IN" | jq -r '.hook_event_name')
TP=$(printf '%s' "$IN" | jq -r '.transcript_path // empty')
if [ -n "$TP" ] && [ -e "$TP" ]; then EX=true; else EX=false; fi
if [ -n "$TP" ]; then SET=true; else SET=false; fi
N=$(find "$HOME/.cursor/projects" -path '*/agent-transcripts/*' -name '*.jsonl' -type f 2>/dev/null | wc -l | tr -d ' ')
printf '{"hook_result":{"event":"%s","transcript_path_set":%s,"transcript_exists":%s,"transcript_files":%s,"pwd":"%s","pwd_physical":"%s"}}\n' "$EV" "$SET" "$EX" "$N" "$(pwd)" "$(pwd -P)" >>"$HOOK_LOG"
exit 0
