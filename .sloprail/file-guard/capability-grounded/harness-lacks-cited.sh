#!/usr/bin/env bash
# A deviation of `kind: harness-lacks` is waived from the user's words because a cited doc or
# recording shows it (adr/capability-grounding), so its cell must cite at least one: a `docs`
# or a `runs` entry. Refuses a cell that has such a deviation and cites neither.
# Contract: stdin is the CheckPayload (a Changeset). exit 0 permits; to refuse, print
# {"reason":"..."} and exit 1.
set -uo pipefail

payload="$(cat)"

refuse() {
  jq -n --arg r "$1" '{reason: $r}'
  exit 1
}

[ "$(printf '%s' "$payload" | jq -r '.event.kind // ""')" = "Changeset" ] ||
  refuse "expected a Changeset event, so the changed files could not be checked"
printf '%s' "$payload" | jq -e '.changeset.files | type == "array"' >/dev/null 2>&1 ||
  refuse "the changeset's files could not be read, so they could not be checked"
count="$(printf '%s' "$payload" | jq -r '.changeset.files | length')" || count=""
case "$count" in '' | *[!0-9]*) refuse "the changeset's files could not be read, so they could not be checked" ;; esac
command -v yq >/dev/null 2>&1 || refuse "yq is not installed, so the capability files could not be checked"

# fine PATH CONTENT: return 0 when no cell has a harness-lacks deviation without a cited doc or run
fine() {
  local path="$1" content="$2" bad
  case "$path" in spec/capabilities/*.yaml) ;; *) return 0 ;; esac
  bad="$(printf '%s' "$content" | yq -o=json '.' 2>/dev/null |
    jq -r '(.providers // {}) | to_entries[] | select(.value | type == "object")
      | select(((.value.deviations // []) | any(.kind == "harness-lacks"))
               and (((.value.docs // []) | length) + ((.value.runs // []) | length)) == 0) | .key')" ||
    { echo "the file could not be parsed"; return 1; }
  [ -z "$bad" ] && return 0
  echo "cell(s) $(printf '%s' "$bad" | tr '\n' ' ')have a kind: harness-lacks deviation and cites no doc or run: cite the doc section or recorded run that shows the harness lacking it, or give the deviation another kind"
  return 1
}

i=0
while [ "$i" -lt "$count" ]; do
  path="$(printf '%s' "$payload" | jq -r --argjson i "$i" '.changeset.files[$i].path')" ||
    refuse "could not read file $i of the changeset, so it could not be checked"
  status="$(printf '%s' "$payload" | jq -r --argjson i "$i" '.changeset.files[$i].status')" ||
    refuse "could not read $path from the changeset, so it could not be checked"
  i=$((i + 1))
  [ "$status" = "D" ] && continue
  content="$(printf '%s' "$payload" | jq -r --argjson i "$((i - 1))" '.changeset.files[$i].newContent')" ||
    refuse "could not read $path from the changeset, so it could not be checked"
  if ! why="$(fine "$path" "$content")"; then
    refuse "$path: $why"
  fi
done
exit 0
