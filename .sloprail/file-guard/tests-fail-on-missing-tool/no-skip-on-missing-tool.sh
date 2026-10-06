#!/usr/bin/env bash
# A Go test whose required external tool (looked up with exec.LookPath) is missing fails; it never calls
# t.Skip, t.Skipf or t.SkipNow for that. Only an explicit opt-in environment gate (A10N_*_TEST) may skip:
# a Skip inside an `if` that reads an A10N_<NAME>_TEST variable is allowed; any other Skip within six
# lines after an exec.LookPath is a skip on a missing tool and is refused. Deterministic, over the changed
# *_test.go files at the head.
set -uo pipefail

payload="$(cat)"

refuse() {
  jq -n --arg r "$1" '{reason: $r}'
  exit 1
}
# the tooling failed, not the change: still refused, but no verdict is cached
refuse_error() {
  jq -n --arg r "$1" '{reason: $r, error: true}'
  exit 1
}

[ "$(printf '%s' "$payload" | jq -r '.event.kind // ""')" = "Changeset" ] ||
  refuse_error "expected a Changeset event, so the changed files could not be checked"
printf '%s' "$payload" | jq -e '.changeset.files | type == "array"' >/dev/null 2>&1 ||
  refuse_error "the changeset's files could not be read, so they could not be checked"
count="$(printf '%s' "$payload" | jq -r '.changeset.files | length')" || count=""
case "$count" in '' | *[!0-9]*) refuse_error "the changeset's files could not be read, so they could not be checked" ;; esac

# skips TEXT: "line: source" for each t.Skip/Skipf/SkipNow that follows an exec.LookPath outside an opt-in gate
skips() {
  awk '
    function indent(s) { match(s, /^[\t ]*/); return RLENGTH }
    { line[NR] = $0; ind[NR] = indent($0) }
    END {
      for (i = 1; i <= NR; i++) {
        if (line[i] ~ /^[ \t]*\/\//) continue
        if (line[i] !~ /\.(Skip|Skipf|SkipNow)\(/) continue
        e = 0
        if (line[i] ~ /^[ \t]*if .*\{.*\.(Skip|Skipf|SkipNow)\(/) e = i
        else for (j = i - 1; j >= 1; j--) { if (line[j] ~ /^[ \t]*$/) continue; if (ind[j] < ind[i]) { e = j; break } }
        ctx = (e ? line[e] : "")
        if (ctx ~ /os\.(Getenv|LookupEnv)\("A10N_[A-Z0-9_]+_TEST"\)/) continue
        start = (e ? e : i) - 6; if (start < 1) start = 1
        for (k = start; k <= i; k++) if (line[k] ~ /exec\.LookPath/) { print i ": " line[i]; break }
      }
    }'
}

bad=""
i=0
while [ "$i" -lt "$count" ]; do
  path="$(printf '%s' "$payload" | jq -r --argjson i "$i" '.changeset.files[$i].path')" ||
    refuse_error "could not read file $i of the changeset, so it could not be checked"
  status="$(printf '%s' "$payload" | jq -r --argjson i "$i" '.changeset.files[$i].status')" ||
    refuse_error "could not read $path from the changeset, so it could not be checked"
  i=$((i + 1))
  [ "$status" = "D" ] && continue
  new="$(printf '%s' "$payload" | jq -r --argjson i "$((i - 1))" '.changeset.files[$i].newContent')" ||
    refuse_error "could not read $path from the changeset, so it could not be checked"
  found="$(printf '%s\n' "$new" | skips)" || refuse_error "could not scan $path for skips"
  [ -z "$found" ] || bad="$bad$(printf '%s\n' "$found" | sed "s#^#- $path:#")"$'\n'
done
[ -z "$bad" ] && exit 0
refuse "adr/tests-fail-on-missing-tool (a test whose required tool is missing fails, it never skips):
${bad}Call t.Fatalf with what to install instead. Only an explicit opt-in environment gate (A10N_*_TEST) may skip."
