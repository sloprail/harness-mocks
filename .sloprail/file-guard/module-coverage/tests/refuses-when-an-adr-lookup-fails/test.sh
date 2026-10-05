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
  jq -es --arg r "$RULE" '[.[] | select(.kind=="FileGuardChecked" and .rule==$r)] | length > 0 and all(.[]; .outcome=="passed")' "$SR_EVENTS_FILE" >/dev/null ||
    { jq -c . "$SR_EVENTS_FILE" >&2; echo "$1: $RULE did not pass (sr-checks exit $ran)" >&2; exit 1; }
}
expect_refused() {   # LABEL SUBSTRING : refused, and the reason says it
  jq -es --arg r "$RULE" --arg s "$2" 'any(.[]; .kind=="FileGuardChecked" and .rule==$r and .outcome=="refused" and (.reason|contains($s)))' "$SR_EVENTS_FILE" >/dev/null ||
    { jq -c . "$SR_EVENTS_FILE" >&2; echo "$1: $RULE did not refuse with a reason saying '$2' (sr-checks exit $ran)" >&2; exit 1; }
}
# inject LABEL EXACT HAS REASON — a fresh commit on BASE (change_for N makes it), the lookup failing: refused with REASON
n=0
inject() {
  n=$((n + 1)); git checkout -q -b "inject$n" "$BASE"; change_for "$n"; c "change $n"
  run_rule "$2" "$3"; expect_refused "$1" "$4"
}
RULE=module-coverage
mkdir -p adr/modules-cover-code internal/a internal/legacy
printf -- '---\nconcern: coverage\nsloprails: [file-guard/module-coverage]\nspace: ["internal/**"]\nexceptions: ["internal/legacy/**"]\n---\n# Every piece of code belongs to a module\n' >adr/modules-cover-code/ADR.md
printf 'concern: "a"\nhome: ["internal/a/**"]\napi: ["internal/a"]\n' >internal/a/module.yaml
printf 'package a\n' >internal/a/a.go; printf 'package legacy\n' >internal/legacy/old.go
c base; BASE=$(git rev-parse HEAD)
change_for() { printf 'package a\n// %s\n' "$1" >internal/a/a.go; }

# control: every file in the space is in a module's home or an exception: passed
git checkout -q -b control "$BASE"; change_for 0; c control
run_rule; expect_passed "control"
# what the exceptions rule exists for is seen when nothing fails: new code under an exception is refused
git checkout -q -b added-under-exception "$BASE"; change_for v; printf 'package legacy\n' >internal/legacy/new.go; c "new code under an exception"
run_rule; expect_refused "new code under an exception" "internal/legacy/new.go is added under an exception"

# each failed lookup refuses, naming it. The changes below add a file under an exception (and put it in
# the tree), the one thing only the failed lookup could let through
git checkout -q -f "$BASE"
change_for() { printf 'package a\n// %s\n' "$1" >internal/a/a.go; printf 'package legacy\n// %s\n' "$1" >"internal/legacy/new$1.go"; }
inject "the ADRs' space"          ''    '.frontmatter.space'      "could not read the space of the ADRs linking file-guard/module-coverage"
inject "the ADRs' exceptions"     ''    '.frontmatter.exceptions' "could not read the exceptions of the ADRs linking file-guard/module-coverage"
inject "the ADR ids"              ''    '.[].id'                  "could not list the ADRs, so it cannot be told whether the exceptions grew"
inject "the base's ADR links"     ''    'index($q)'               "at the range's base could not be read for its links"
inject "the base's exceptions"    '.exceptions // [] | .[]' '' "could not read the exceptions of adr/modules-cover-code/ADR.md at the range's base"
