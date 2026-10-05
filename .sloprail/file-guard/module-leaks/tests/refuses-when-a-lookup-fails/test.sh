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

# each failed lookup refuses, naming it. subjects.sh runs first, so what only find-leaks.sh reads is
# reached when the lookup is one only it makes
inject "the ADRs' exceptions"       ''   'frontmatter.exceptions'        "the ADRs' exceptions could not be read"
inject "the lines the range adds"   ''   '.changeset.files[] | select(.status != "D")' "the lines the range adds could not be worked out"
inject "the module list"            '.[]' ''                             "could not list the modules, so their leaks cannot be searched for"
inject "a module's directory"       '.dir' ''                            "could not read a module's directory, so its leaks cannot be searched for"
inject "a module's home"            ''   '.home[]'                       "could not read the home of module internal/a, so its leaks cannot be found"
inject "a candidate's line number"  ''   'tonumber'                      "the candidates of internal/a could not be read as path, line and text"
inject "the candidate count"        'length' ''                          "could not count the candidates of internal/a"
inject "the subject's key"          ''   'split("\n")'                   "could not build the subject of module internal/a"
