#!/usr/bin/env bash
# A scripted agent: claude-mock runs it once per turn; it prints raw stream-json lines.
n=$(grep -c "\"tool_result\"" "$A10N_MOCK_SESSION_FILE" 2>/dev/null); n=${n:-0}
tu() { jq -nc --arg id "$1" --arg name "$2" --argjson input "$3" '{type:"assistant",uuid:("u-"+$id),message:{role:"assistant",stop_reason:null,content:[{type:"tool_use",id:$id,name:$name,input:$input}]}}'; }
write_() { tu "$1" Write "$(jq -nc --arg p "$PWD/$2" --arg c "$3" '{file_path:$p,content:$c}')"; }
finish() { echo '{"type":"result","subtype":"success","result":"done","is_error":false}'; }
case $n in
  0) write_ t1 claude-mock/snapshots/runs/r/samples/20240101-000000/events.jsonl '{"e":2}' ;;
  1) write_ t2 claude-mock/snapshots/runs/r/setup/prompt.txt 'say hello' ;;
  2) write_ t3 claude-mock/snapshots/capture.sh '#!/bin/sh' ;;
  *) finish ;;
esac
