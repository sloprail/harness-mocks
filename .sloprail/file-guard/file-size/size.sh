#!/usr/bin/env bash
# The file-guard half of adr/file-size: every changed Go file in the committed
# changeset stays within its limit or its legacy ceiling (size-lib.sh holds the
# rule). Follows the skill's check-template.sh: anything but a readable Changeset
# is a refusal, never a pass.
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

. "${SR_GUARDRAIL_DIR:-.}/size-lib.sh"
size_load

problems=""
i=0
while [ "$i" -lt "$count" ]; do
  path="$(printf '%s' "$payload" | jq -r --argjson i "$i" '.changeset.files[$i].path')" ||
    refuse "could not read file $i of the changeset, so it could not be checked"
  status="$(printf '%s' "$payload" | jq -r --argjson i "$i" '.changeset.files[$i].status')" ||
    refuse "could not read $path from the changeset, so it could not be checked"
  content="$(printf '%s' "$payload" | jq -r --argjson i "$i" '.changeset.files[$i].newContent // ""')" ||
    refuse "could not read $path from the changeset, so it could not be checked"
  i=$((i + 1))
  [ "$status" = "D" ] && continue
  p="$(size_problem "$path" "$content")"
  [ -z "$p" ] || problems="${problems}- $p"$'\n'
done
[ -z "$problems" ] && exit 0
refuse "adr/file-size (Go files stay small):
${problems}"
