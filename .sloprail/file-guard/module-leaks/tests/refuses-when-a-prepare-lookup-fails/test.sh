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
  jq -es '[.[] | select(.kind=="FileGuardChecked" and .rule=="module-leaks")] | length > 0 and all(.[]; .outcome=="passed")' "$SR_EVENTS_FILE" >/dev/null ||
    { jq -c . "$SR_EVENTS_FILE" >&2; echo "$1: $RULE did not pass (sr-checks exit $ran)" >&2; exit 1; }
}
expect_refused() {   # LABEL SUBSTRING : refused, and the reason says it
  jq -es --arg s "$2" 'any(.[]; .kind=="FileGuardChecked" and .rule=="module-leaks" and .outcome=="refused" and (.reason|contains($s)))' "$SR_EVENTS_FILE" >/dev/null ||
    { jq -c . "$SR_EVENTS_FILE" >&2; echo "$1: $RULE did not refuse with a reason saying '$2' (sr-checks exit $ran)" >&2; exit 1; }
}
# inject LABEL EXACT HAS REASON — a fresh commit on BASE (change_for N makes it), the lookup failing: refused with REASON
n=0
inject() {
  n=$((n + 1)); git checkout -q -b "inject$n" "$BASE"; change_for "$n"; c "change $n"
  run_rule "$2" "$3"; expect_refused "$1" "$4"
}
RULE=module-leaks
# the judge is a mock that passes: what is under test is what feeds it (subjects.sh, find-leaks.sh, leaks-lib.sh)
export SR_CHECKS_JUDGE_MOCKS='{"file-guard/module-leaks/leak-or-use":"'"$SR_TEST_CASE_DIR"'/judge.sh"}'
mkdir -p internal/a internal/other
printf 'concern: "a owns a"\nhome: ["internal/a/**"]\napi: ["internal/a"]\n' >internal/a/module.yaml
# a's own search: where its logic appears (anywhere in the tree), in `git grep -n` format
printf '#!/usr/bin/env bash\ngit grep -n -I -E "DoA[(]" -- "*.go" || true\n' >internal/a/candidates.sh; chmod +x internal/a/candidates.sh
printf 'package a\n\nfunc DoA() {}\n' >internal/a/a.go; printf 'package other\n' >internal/other/o.go
c base; BASE=$(git rev-parse HEAD)
# the change adds a line outside a's home that matches its search: a candidate for the judge
change_for() { printf 'package other\n\nfunc f() { DoA(%s) }\n' "$1" >internal/other/o.go; }

# control: a candidate is found and handed to the (mock) judge, which passes it
git checkout -q -b control "$BASE"; change_for 0; c control
run_rule; expect_passed "control"

# what only find-leaks.sh runs (it is the prepare of the same rule; subjects.sh, which runs first, never makes
# these lookups): the candidates written for the judge, the group recorded, the groups counted, the context built
inject "the candidates written for the judge" ''   '.[] | "\(.path)'   "could not write the candidates of internal/a for the judge"
inject "the group recorded"                   ''   '$m.concern'      "could not record the candidates of internal/a for the judge"
FAIL_JQ_STDIN='"matches":' inject "the groups counted" 'length' ''   "could not count the modules with candidates left"
inject "the judge's context"                  ''   '{additionalContext' "could not build the judge's context"

# the kept candidates (leaks-lib.sh leak_search): a kept entry that cannot be copied is a failed lookup, not
# "the search found nothing". The entry is written by hand, so the rule takes the cache path
cache_entry() {   # writes the kept candidates of internal/a for HEAD's tree
  local d; d="$(git rev-parse --path-format=absolute --git-common-dir)/sloprail-candidates-cache"; mkdir -p "$d"
  printf 'internal/other/o.go:3:func f() { DoA(%s) }\n' "$1" >"$d/$(git rev-parse 'HEAD^{tree}')-internal_a"
}
git checkout -q -b cache-control "$BASE"; change_for kc; c "cache control"; cache_entry kc
run_rule; expect_passed "a kept entry is used"
git checkout -q -b cache-copy "$BASE"; change_for kp; c "cache copy"; cache_entry kp
FAIL_CP_ARGS=sloprail-candidates-cache run_rule
expect_refused "the kept candidates copied" "the kept candidates of internal/a could not be read, so its leaks cannot be found"
# the background prefetch job must not print a refusal of its own: leak_left refuses once, in the open (the
# reason is the one refusal; the old background refusal printed a second JSON object to the same stdout)
git checkout -q -b cache-dir "$BASE"; change_for kd; c "cache dir"
FAIL_GIT_ARGS='--path-format=absolute --git-common-dir' run_rule
expect_refused "the git directory" "the git directory of the tree could not be found, so the candidates of internal/a cannot be kept or read"
