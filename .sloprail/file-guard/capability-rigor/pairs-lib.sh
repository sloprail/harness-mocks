#!/usr/bin/env bash
# rigor_pairs — the (capability, harness) pairs this changeset touches, one
# per line: "<id>/<harness>\t<cell json>\t<capability json>". Touched: the
# capability file changed in that harness's cell (cells.sh; its statement, or
# the whole file, changing touches every providing harness); a marker naming it changed
# (sr:capability → every harness; sr:provides/sr:proves <id>/<h> → that harness);
# or a snapshot it cites for <h> changed (a run, or the MANIFEST: a re-frozen
# doc). Source after changeset.sh, spec.sh and snapshots.sh; no event logic.
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/cells.sh"
rigor_pairs() {
  local caps changed fqns c id h cell hit r touched
  load_spec capabilities; caps="$SPEC"
  changed="$(cs '.changeset.files[].path')"
  fqns="$(cs '.changeset.files[] | ((.newMarkers // []) + (.oldMarkers // []))[] | select(.kind == "capability" or .kind == "provides" or .kind == "proves") | .fqn')"
  while IFS= read -r c; do
    [ -n "$c" ] || continue
    id="$(jq -r '.id' <<<"$c")"
    touched=""
    printf '%s\n' "$changed" | grep -Fxq "spec/capabilities/$id.yaml" && touched="$(touched_harnesses "spec/capabilities/$id.yaml")"
    for h in $(jq -r '.doc.providers // {} | to_entries[] | select(.value | type == "object") | .key' <<<"$c"); do
      cell="$(jq -c --arg h "$h" '.doc.providers[$h]' <<<"$c")"
      hit=0
      printf '%s\n' "$touched" | grep -Fxq -e '*' -e "$h" && hit=1
      printf '%s\n' "$fqns" | grep -Fxq -e "$id" -e "$id/$h" && hit=1
      for r in $(jq -r '.runs[]' <<<"$cell"); do printf '%s\n' "$changed" | grep -q "^$r/" && hit=1; done
      printf '%s\n' "$changed" | grep -Fxq "$h-mock/snapshots/MANIFEST.yaml" && hit=1
      [ "$hit" = 1 ] && printf '%s/%s\t%s\t%s\n' "$id" "$h" "$cell" "$c"
    done
  done < <(jq -c '.[]' <<<"$caps")
}
