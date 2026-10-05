#!/usr/bin/env bash
# Turn two: the user answered "lgtm"; the agent makes a change and cites that reply. CHANGE=proposed (the default)
# makes the change the assistant had proposed; anything else makes an unrelated one.
n=$(grep -c "\"tool_result\"" "$A10N_MOCK_SESSION_FILE" 2>/dev/null); n=${n:-0}
tu() { jq -nc --arg id "$1" --arg name "$2" --argjson input "$3" '{type:"assistant",uuid:("u-"+$id),message:{role:"assistant",stop_reason:null,content:[{type:"tool_use",id:$id,name:$name,input:$input}]}}'; }
bash_() { tu "$1" Bash "$(jq -nc --arg c "$2" '{command:$c}')"; }
write_() { tu "$1" Write "$(jq -nc --arg p "$2" --arg c "$3" '{file_path:$p,content:$c}')"; }
finish() { echo '{"type":"result","subtype":"success","result":"done","is_error":false}'; }
RUN='sr-checks run --base $(git rev-list --max-parents=0 HEAD) --head HEAD'
GIT='git -c user.name=t -c user.email=t@t'
case "${CHANGE:-proposed}" in proposed) F=spec/invariants/queue-keeps-order.yaml ;; *) F=spec/invariants/cache-never-stale.yaml ;; esac
case $n in
  0) write_ w1 "$F" $'statement: >-\n  A cache never serves a stale entry.\n' ;;
  1) bash_ a1 "git add -A" ;;
  2) bash_ b1 "$GIT commit -q -m 'change the project' -m 'Sloprail-Cites-User: lgtm'" ;;
  3) bash_ c1 "$RUN" ;;
  *) finish ;;
esac
