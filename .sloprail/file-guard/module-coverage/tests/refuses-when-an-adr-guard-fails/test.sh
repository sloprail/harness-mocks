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
  [ -n "\${FAIL_JQ_EXACT:-}" ] && [ "\$a" = "\$FAIL_JQ_EXACT" ] && {
    # an exact program shared by two scripts: fail only when the input holds FAIL_JQ_STDIN (when set)
    [ -n "\${FAIL_JQ_STDIN:-}" ] || exit 3
    in="\$(cat)"; case "\$in" in *"\$FAIL_JQ_STDIN"*) exit 3 ;; esac
    printf '%s' "\$in" | "$REAL_JQ" "\$@"; exit \$?
  }
  [ -n "\${FAIL_JQ_HAS:-}" ] && case "\$a" in *"\$FAIL_JQ_HAS"*) exit 3 ;; esac
done
exec "$REAL_JQ" "\$@"
SHIM
# git and cp shims: fail when the joined arguments contain $FAIL_GIT_ARGS / $FAIL_CP_ARGS, else run the real one
for tool in git cp; do
  up="$(printf '%s' "$tool" | tr a-z A-Z)"; real="$(command -v "$tool")"
  cat >"$TMPDIR/shim/$tool" <<SHIM
#!/usr/bin/env bash
[ -n "\${FAIL_${up}_ARGS:-}" ] && case "\$*" in *"\$FAIL_${up}_ARGS"*) exit 3 ;; esac
exec "$real" "\$@"
SHIM
  chmod +x "$TMPDIR/shim/$tool"
done
chmod +x "$TMPDIR/shim/jq"
git init -q .
c() { git add -A && git -c user.name=t -c user.email=t@t commit -q -m "$1"; }
# run_rule [EXACT HAS] — judge BASE..HEAD, the jq failing when asked to; the rule's events land in $SR_EVENTS_FILE
run_rule() {
  : > "$SR_EVENTS_FILE"
  PATH="$TMPDIR/shim:$PATH" FAIL_JQ_EXACT="${1:-}" FAIL_JQ_HAS="${2:-}" sr-checks run --base "$BASE" --head HEAD >/dev/null 2>&1 && ran=0 || ran=$?
}
expect_passed() {   # LABEL
  jq -es '[.[] | select(.kind=="FileGuardChecked" and .rule=="module-coverage")] | length > 0 and all(.[]; .outcome=="passed")' "$SR_EVENTS_FILE" >/dev/null ||
    { jq -c . "$SR_EVENTS_FILE" >&2; echo "$1: $RULE did not pass (sr-checks exit $ran)" >&2; exit 1; }
}
expect_refused() {   # LABEL SUBSTRING : refused, and the reason says it
  jq -es --arg s "$2" 'any(.[]; .kind=="FileGuardChecked" and .rule=="module-coverage" and .outcome=="refused" and (.reason|contains($s)))' "$SR_EVENTS_FILE" >/dev/null ||
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

# the changes below add a file under an exception (and put it in the tree), the one thing only the failed lookup could let through
change_for() { printf 'package a\n// %s\n' "$1" >internal/a/a.go; printf 'package legacy\n// %s\n' "$1" >"internal/legacy/new$1.go"; }
git checkout -q -b control "$BASE"; change_for 0; c control
run_rule; expect_refused "control: the new file under the exception" "internal/legacy/new0.go is added under an exception"

# the guards before the lookups: a failed type check of the loaded ADRs, a failed read of the modules' concern lines
inject "the loaded ADRs' type"    ''    'type == "array"'         "the ADRs linking file-guard/module-coverage could not be loaded, so module coverage cannot be checked"
inject "the modules' concern"     ''    'tostring | gsub'         "could not read the modules' concern lines"
