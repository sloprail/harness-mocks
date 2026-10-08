#!/usr/bin/env bash
# A scripted agent: claude-mock runs it once per turn; it prints raw stream-json lines.
n=$(grep -c "\"tool_result\"" "$A10N_MOCK_SESSION_FILE" 2>/dev/null); n=${n:-0}
tu() { jq -nc --arg id "$1" --arg name "$2" --argjson input "$3" '{type:"assistant",uuid:("u-"+$id),message:{role:"assistant",stop_reason:null,content:[{type:"tool_use",id:$id,name:$name,input:$input}]}}'; }
bash_() { tu "$1" Bash "$(jq -nc --arg c "$2" '{command:$c}')"; }
finish() { echo '{"type":"result","subtype":"success","result":"done","is_error":false}'; }
case $n in
  0) bash_ t1 "rm claude-mock/snapshots/runs/r/samples/20240101-000000/events.jsonl" ;;
  1) bash_ t2 "sh claude-mock/snapshots/capture.sh run r" ;;
  2) bash_ t3 "rm claude-mock/snapshots/runs/r/setup/prompt.txt" ;;
  *) finish ;;
esac
