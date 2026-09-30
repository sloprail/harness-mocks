#!/usr/bin/env bash
# Catalog diffing for the funnel's subjects scripts. Source after changeset.sh.

# changed_entries FILE KEY — the entries of list KEY in catalog FILE that this
# changeset added, changed or removed, as a JSON array of
# {id, change: added|changed|removed, before, after}. Compared by id, on the
# parsed YAML, so reformatting a file changes nothing.
changed_entries() {
  local file="$1" key="$2" entry old new
  entry="$(changed_file "$file")"
  [ "$entry" != "null" ] || { printf '[]'; return 0; }
  old="$(yaml_str_json "$(printf '%s' "$entry" | jq -r '.oldContent // ""')")"
  new="$(yaml_str_json "$(printf '%s' "$entry" | jq -r '.newContent // ""')")"
  jq -n -c --argjson o "${old:-null}" --argjson n "${new:-null}" --arg k "$key" '
    def byid(x): ((x // {})[$k] // []) | map({key: .id, value: .}) | from_entries;
    byid($o) as $b | byid($n) as $a
    | [ ($b + $a | keys[]) as $id
        | {id: $id, before: $b[$id], after: $a[$id]}
        | select(.before != .after)
        | .change = (if .before == null then "added" elif .after == null then "removed" else "changed" end) ]'
}
