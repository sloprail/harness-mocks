#!/usr/bin/env bash
# A Go test whose required external tool (looked up with exec.LookPath) is missing fails; it never calls
# t.Skip for that. Only an explicit opt-in environment gate (A10N_*_TEST) may skip.
#
# The Go is read from its types by tools/skipcheck, never from its text: it type-checks each package that
# has a changed *_test.go (a root-level file's directory is "."), a package and its _test package together,
# and when any code of the package (a function, a variable initializer, TestMain) names os/exec.LookPath
# (an alias or a dot import counts) it refuses every call or method value of Skip, Skipf or SkipNow in the
# package, a generic, interface or type-parameter method included. A skip directly under
# `if os.Getenv("A10N_<NAME>_TEST") <op> <constant>` (that condition and nothing else) is the opt-in gate, and
# a test of such a package may not set an A10N_*_TEST name (os.Setenv, os.Unsetenv, t.Setenv).
# Only a lookup inside the test package counts, not one through another package ("a test package that looks up
# an external tool"), and only a named os/exec.LookPath counts: exec.Command is not treated as a lookup.
# A checker that cannot build or run is refuse_error, not a verdict.
set -uo pipefail

payload="$(cat)"

refuse() {
  jq -n --arg r "$1" '{reason: $r}'
  exit 1
}
# refuse_error MESSAGE — the tooling failed, not the change: still refused, but "error": true makes the
# engine store no verdict, so the next run tries again (see _lib/changeset.sh)
refuse_error() {
  jq -n --arg r "$1" '{reason: $r, error: true}'
  exit 1
}

[ "$(printf '%s' "$payload" | jq -r '.event.kind // ""')" = "Changeset" ] ||
  refuse_error "expected a Changeset event, so the changed files could not be checked"
printf '%s' "$payload" | jq -e '.changeset.files | type == "array"' >/dev/null 2>&1 ||
  refuse_error "the changeset's files could not be read, so they could not be checked"

# the directories of the changed (not deleted) test files
dirs="$(printf '%s' "$payload" | jq -r '[.changeset.files[] | select(.status != "D" and (.path | endswith("_test.go"))) | .path | if contains("/") then sub("/[^/]*$"; "") else "." end] | unique | .[]')" ||
  refuse_error "could not read the changed test files from the changeset, so they could not be checked"
[ -n "$dirs" ] || exit 0

[ -d "${SR_TREE:-}/tools/skipcheck" ] ||
  refuse_error "tools/skipcheck is not in the committed tree, so the tests cannot be checked"
work="$(mktemp -d)" || refuse_error "could not make a directory for the skip checker"
build="$(cd "$SR_TREE" && go build -o "$work/skipcheck" ./tools/skipcheck 2>&1)" ||
  refuse_error "could not build tools/skipcheck, so the tests cannot be checked: $build"

args=()
for d in $dirs; do args+=("$SR_TREE/$d"); done
out="$("$work/skipcheck" "${args[@]}" 2>"$work/err")"
rc=$?
case "$rc" in
  0) exit 0 ;;
  1) refuse "adr/tests-fail-on-missing-tool (a Go test whose required external tool is missing fails, it never calls t.Skip for that; only an explicit opt-in environment gate A10N_*_TEST may skip):
${out//$SR_TREE\//}
Call t.Fatalf with what to install instead." ;;
  *) refuse_error "skipcheck failed ($rc), so the tests cannot be checked: $(cat "$work/err")" ;;
esac
