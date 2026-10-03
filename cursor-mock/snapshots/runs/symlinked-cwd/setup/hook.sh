#!/bin/sh
# One hook for every event, run from a symlinked working directory. It logs each
# payload and, for each one: whether the file named by its transcript_path exists
# at that moment, how many session transcript files exist under the config
# directory at that moment (whatever the payload names), what directory the
# hook itself ran in (logical and physical), and which directory the project
# folder under the config directory is keyed by: the resolved workspace
# (key_resolved) or the symlink the process was started from, which sits beside
# the workspace under the name "link" (key_symlink). The folder is named after
# a path with every non-alphanumeric character as "-".
IN=$(cat)
printf '%s\n' "$IN" >>"$HOOK_LOG"
EV=$(printf '%s' "$IN" | jq -r '.hook_event_name')
TP=$(printf '%s' "$IN" | jq -r '.transcript_path // empty')
if [ -n "$TP" ] && [ -e "$TP" ]; then EX=true; else EX=false; fi
if [ -n "$TP" ]; then SET=true; else SET=false; fi
N=$(find "$HOME/.cursor/projects" -path '*/agent-transcripts/*' -name '*.jsonl' -type f 2>/dev/null | wc -l | tr -d ' ')
RESOLVED=$(pwd -P)
SYMLINK="$(dirname "$RESOLVED")/link"
ENC_RESOLVED=$(printf '%s' "${RESOLVED#/}" | sed 's#[^A-Za-z0-9]#-#g')
ENC_SYMLINK=$(printf '%s' "${SYMLINK#/}" | sed 's#[^A-Za-z0-9]#-#g')
KR=false; KS=false
case "$(find "$HOME/.cursor/projects" -mindepth 1 -maxdepth 1 -type d 2>/dev/null)" in *"/projects/$ENC_RESOLVED") KR=true ;; esac
case "$(find "$HOME/.cursor/projects" -mindepth 1 -maxdepth 1 -type d 2>/dev/null)" in *"/projects/$ENC_SYMLINK") KS=true ;; esac
printf '{"hook_result":{"event":"%s","transcript_path_set":%s,"transcript_exists":%s,"transcript_files":%s,"pwd":"%s","pwd_physical":"%s","key_resolved":%s,"key_symlink":%s}}\n' "$EV" "$SET" "$EX" "$N" "$(pwd)" "$(pwd -P)" "$KR" "$KS" >>"$HOOK_LOG"
exit 0
