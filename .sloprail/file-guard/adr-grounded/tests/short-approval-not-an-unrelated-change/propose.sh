#!/usr/bin/env bash
# Turn one of the session: the assistant proposes a change in prose, then says something else before the user
# answers, so the proposal sits a few messages before the reply.
text() { jq -nc --arg id "$1" --arg t "$2" '{type:"assistant",uuid:("u-"+$id),message:{role:"assistant",stop_reason:null,content:[{type:"text",text:$t}]}}'; }
text prop "I propose to add an ADR, retry-on-timeout: a timed-out call is retried once. Good to go?"
text aside "That is a small edit, a single file."
text aside2 "Waiting for your answer."
echo '{"type":"result","subtype":"success","result":"done","is_error":false}'
