#!/usr/bin/env bash
# For each spec/capabilities/<id>.yaml:
#   - keys: a non-empty string `statement`, and `providers`
#   - providers: a cell for EVERY harness mock (<h>-mock/ dirs), each `false` or
#     {docs: [≥1], runs: [≥1]}; no cell for a harness that does not exist
#   - exactly one `// sr:capability <id>`, under core/
#   - each harness set to a cell: ≥1 `// sr:provides <id>/<h>` under <h>-mock/,
#     and ≥1 `// sr:proves <id>/<h>` in a *_test.go
# And back: every sr:capability / sr:provides / sr:proves <x>/<h> names a
# capability, and a harness whose cell is not false.
set -uo pipefail
payload="$(cat)"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/changeset.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/spec.sh"
load_spec capabilities; caps="$SPEC"
load_markers capability; impl="$MARKERS"
load_markers provides; provides="$MARKERS"
load_markers proves; proves="$(printf '%s\n' "$MARKERS" | awk -F'\t' 'NF && $2 ~ /\//')"
hs="$(harnesses)"

problems=""
add() { problems="${problems}- $1"$'\n'; }
# `// "missing"` would read a false cell as missing: jq's // treats false as absent.
cell() { jq -c --arg id "$1" --arg h "$2" '[.[] | select(.id == $id)][0].doc.providers | if has($h) then .[$h] else "missing" end' <<<"$caps"; }

while IFS= read -r c; do
  [ -n "$c" ] || continue
  id="$(jq -r '.id' <<<"$c")"
  kebab "$id" || add "spec/capabilities/$id.yaml: the file name must be kebab-case"
  jq -e '(.doc | type) == "object" and ((.doc | keys) == ["providers", "statement"])
         and (.doc.statement | type) == "string" and (.doc.statement | length) > 0
         and (.doc.providers | type) == "object"' <<<"$c" >/dev/null ||
    { add "spec/capabilities/$id.yaml must hold exactly 'statement' (a non-empty string) and 'providers' (a map)"; continue; }
  for h in $(jq -r '.doc.providers | keys[]' <<<"$c"); do
    printf '%s\n' "$hs" | grep -Fxq -- "$h" || add "capability '$id' has a cell for '$h', but there is no $h-mock/"
  done
  n="$(printf '%s\n' "$impl" | awk -F'\t' -v id="$id" '$2 == id' | grep -c .)"
  [ "$n" -eq 1 ] || add "capability '$id' needs exactly one // sr:capability $id, in core/ (found $n)"
  for h in $hs; do
    v="$(cell "$id" "$h")"
    case "$v" in
      '"missing"') add "capability '$id' has no cell for '$h': set it to false, or {docs, runs}" ;;
      false) ;;
      *)
        jq -e '(type) == "object" and ((keys) == ["docs", "runs"]) and (.docs | type) == "array" and (.docs | length) > 0
               and (.runs | type) == "array" and (.runs | length) > 0' <<<"$v" >/dev/null ||
          add "capability '$id' × '$h' must be false or {docs: [≥1], runs: [≥1]}"
        printf '%s\n' "$provides" | awk -F'\t' -v f="$id/$h" '$2 == f' | grep -q . ||
          add "capability '$id' is provided by '$h' but no $h-mock/ code carries // sr:provides $id/$h"
        printf '%s\n' "$proves" | awk -F'\t' -v f="$id/$h" '$2 == f && $1 ~ /_test\.go$/' | grep -q . ||
          add "capability '$id' is provided by '$h' but no test carries // sr:proves $id/$h"
        ;;
    esac
  done
done < <(jq -c '.[]' <<<"$caps")

while IFS=$'\t' read -r path id; do
  [ -n "$path" ] || continue
  jq -e --arg id "$id" 'any(.[]; .id == $id)' <<<"$caps" >/dev/null || add "$path: sr:capability '$id' names no spec/capabilities/$id.yaml"
  case "$path" in core/*) ;; *) add "$path: capability '$id' is implemented outside core/" ;; esac
done <<<"$impl"
check_ref() {   # KIND PATH FQN WHERE-GLOB
  local kind="$1" path="$2" fqn="$3" id="${3%%/*}" h="${3#*/}" v
  case "$fqn" in */*) ;; *) add "$path: sr:$kind '$fqn' must be <capability>/<harness>"; return ;; esac
  v="$(cell "$id" "$h")"
  case "$v" in '"missing"' | false) add "$path: sr:$kind $fqn, but '$id' has no cell for '$h' set to {docs, runs}"; return ;; esac
  case "$kind:$path" in
    provides:"$h"-mock/*) ;; provides:*) add "$path: sr:provides $fqn must sit under $h-mock/" ;;
    proves:*_test.go) ;; proves:*) add "$path: sr:proves belongs on a test, in a *_test.go" ;;
  esac
}
while IFS=$'\t' read -r path fqn; do [ -n "$path" ] && check_ref provides "$path" "$fqn"; done <<<"$provides"
while IFS=$'\t' read -r path fqn; do [ -n "$path" ] && check_ref proves "$path" "$fqn"; done <<<"$proves"

[ -z "$problems" ] && exit 0
refuse "Capabilities not implemented, adapted or proven (adr/capability-once):
${problems}"
