#!/bin/sh
# One hook for the events that can hand the agent context. It logs the payload
# it read and answers with an additional_context naming the event and the
# argument of the hook entry that ran it (CTX-<event>-<arg>), so what reaches
# the conversation can be told apart by hook.
IN=$(cat)
printf '%s\n' "$IN" >>"$HOOK_LOG"
EV=$(printf '%s' "$IN" | jq -r '.hook_event_name')
printf '{"additional_context":"CTX-%s-%s"}\n' "$EV" "$1"
printf '{"hook_result":{"event":"%s","arg":"%s","exit":0}}\n' "$EV" "$1" >>"$HOOK_LOG"
exit 0
