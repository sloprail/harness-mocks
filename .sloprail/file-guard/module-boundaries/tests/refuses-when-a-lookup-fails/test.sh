#!/usr/bin/env bash
set -euo pipefail

# A rule refuses when it cannot work out what to check; it never passes on a failed lookup (#204).
# The CI path, no agent turn: `sr-checks run` judges a committed range with the project's rules. Each
# lookup of the rule is made to FAIL in turn by a `jq` shim put first on PATH for that run only: it exits 3
# when an argument equals $FAIL_JQ_EXACT or contains $FAIL_JQ_HAS (a piece of the lookup's own program),
# else it runs the real jq. The rule must then refuse, saying which lookup failed. A control run, with no
# failure injected, shows the same range is otherwise judged and passes.
# The verdicts are kept by content, so every injection is its own commit (a stored verdict is replayed).
YQ="$(PATH="/opt/homebrew/bin:/usr/local/bin:/usr/bin" command -v yq)" || { echo "needs yq on the machine running the case" >&2; exit 1; }
mkdir -p "$TMPDIR/tools" "$TMPDIR/shim" && ln -sf "$YQ" "$TMPDIR/tools/yq" && export PATH="$PATH:$TMPDIR/tools"
REAL_JQ="$(command -v jq)"
cat >"$TMPDIR/shim/jq" <<SHIM
#!/usr/bin/env bash
for a in "\$@"; do
  [ -n "\${FAIL_JQ_EXACT:-}" ] && [ "\$a" = "\$FAIL_JQ_EXACT" ] && exit 3
  [ -n "\${FAIL_JQ_HAS:-}" ] && case "\$a" in *"\$FAIL_JQ_HAS"*) exit 3 ;; esac
done
exec "$REAL_JQ" "\$@"
SHIM
chmod +x "$TMPDIR/shim/jq"
git init -q .
c() { git add -A && git -c user.name=t -c user.email=t@t commit -q -m "$1"; }
# run_rule [EXACT HAS] — judge BASE..HEAD, the jq failing when asked to; the rule's events land in $SR_EVENTS_FILE
run_rule() {
  : > "$SR_EVENTS_FILE"
  PATH="$TMPDIR/shim:$PATH" FAIL_JQ_EXACT="${1:-}" FAIL_JQ_HAS="${2:-}" sr-checks run --base "$BASE" --head HEAD >/dev/null 2>&1 && ran=0 || ran=$?
}
expect_passed() {   # LABEL
  jq -es '[.[] | select(.kind=="FileGuardChecked" and .rule=="module-boundaries")] | length > 0 and all(.[]; .outcome=="passed")' "$SR_EVENTS_FILE" >/dev/null ||
    { jq -c . "$SR_EVENTS_FILE" >&2; echo "$1: $RULE did not pass (sr-checks exit $ran)" >&2; exit 1; }
}
expect_refused() {   # LABEL SUBSTRING : refused, and the reason says it
  jq -es --arg s "$2" 'any(.[]; .kind=="FileGuardChecked" and .rule=="module-boundaries" and .outcome=="refused" and (.reason|contains($s)))' "$SR_EVENTS_FILE" >/dev/null ||
    { jq -c . "$SR_EVENTS_FILE" >&2; echo "$1: $RULE did not refuse with a reason saying '$2' (sr-checks exit $ran)" >&2; exit 1; }
  case "$1" in control*|violation|implemented*|new\ code*) return 0 ;; esac
  jq -es --arg s "$2" 'any(.[]; .kind=="FileGuardChecked" and .outcome=="refused" and (.reason|contains($s)) and (.reason|contains("could not be evaluated")))' "$SR_EVENTS_FILE" >/dev/null ||
    { jq -c . "$SR_EVENTS_FILE" >&2; echo "$1: refused as a verdict, not reported as an error (no verdict to cache)" >&2; exit 1; }
}
# inject LABEL EXACT HAS REASON — a fresh commit on BASE (change_for N makes it), the lookup failing: refused with REASON
n=0
inject() {
  n=$((n + 1)); git checkout -q -b "inject$n" "$BASE"; change_for "$n"; c "change $n"
  run_rule "$2" "$3"; expect_refused "$1" "$4"
}
RULE=module-boundaries
# a fake `go` (no toolchain needed): the module is example.test/m and `go list` prints the committed go.list
cat >"$TMPDIR/tools/go" <<'GO'
#!/usr/bin/env bash
case "$*" in
  "list -m") echo example.test/m ;;
  list*) cat go.list ;;
esac
GO
chmod +x "$TMPDIR/tools/go"
mkdir -p internal/a internal/b
printf 'module example.test/m\n' >go.mod
printf 'concern: "a"\nhome: ["internal/a/**"]\napi: ["internal/a"]\n' >internal/a/module.yaml
printf 'package a\n' >internal/a/a.go; printf 'package b\n' >internal/b/b.go
# b imports a's api: no violation
printf 'example.test/m/internal/b example.test/m/internal/a\nexample.test/m/internal/a\n' >go.list
c base; BASE=$(git rev-parse HEAD)
change_for() { printf 'package b\n// %s\n' "$1" >internal/b/b.go; }

# control: nothing leaks, nothing fails: passed
git checkout -q -b control "$BASE"; change_for 0; c control
run_rule; expect_passed "control"
# the violation the rule exists for is seen when nothing fails (so the failures below are not a bad fixture)
git checkout -q -b violation "$BASE"; change_for v
printf 'example.test/m/internal/b example.test/m/internal/a/inner\nexample.test/m/internal/a\n' >go.list; c violation
run_rule; expect_refused "violation" "internal/b imports internal/a/inner, inside module internal/a but not its api"
# the recovery the refusal names: b imports the api package instead; the same range then passes
printf 'example.test/m/internal/b example.test/m/internal/a\nexample.test/m/internal/a\n' >go.list; c "b imports a's api"
run_rule; expect_passed "the violation fixed (b imports a's api)"
git checkout -q -f "$BASE"
# the violation stays in the tree for every failure below: a lookup that fails must refuse, not pass over it
printf 'example.test/m/internal/b example.test/m/internal/a/inner\nexample.test/m/internal/a\n' >go.list; c "b reaches into a"; BASE=$(git rev-parse HEAD)

inject "the module list"        '.[]'     ''         "could not list the modules, so module boundaries cannot be checked"
inject "the module count"       'length'  ''         "could not count the modules, so module boundaries cannot be checked"
inject "a module's directory"   '.dir'    ''         "could not read a module's directory, so module boundaries cannot be checked"
inject "a module's home"        ''        '.home[]'  "could not read the home of module internal/a, so module boundaries cannot be checked"
inject "a module's api"         ''        '.api[]'   "could not read the api of module internal/a, so module boundaries cannot be checked"
