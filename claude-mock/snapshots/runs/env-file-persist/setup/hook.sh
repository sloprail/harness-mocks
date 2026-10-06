#!/bin/sh
# SessionStart writes an export to CLAUDE_ENV_FILE; every event logs its payload and whether it was given the file.
IN=$(cat)
printf '%s\n' "$IN" >>"$HOOK_LOG"
EV=$(printf '%s' "$IN" | jq -r '.hook_event_name')
printf '{"env_file_event":"%s","env_file_set":"%s"}\n' "$EV" "${CLAUDE_ENV_FILE:+yes}" >>"$HOOK_LOG"
if [ "$EV" = SessionStart ]; then
  printf 'export FROM_ENV_FILE=persisted\nexport PATH_EXTRA=$PWD/extra\n' >>"$CLAUDE_ENV_FILE"
fi
exit 0
