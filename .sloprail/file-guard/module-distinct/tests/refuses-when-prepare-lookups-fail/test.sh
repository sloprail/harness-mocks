#!/usr/bin/env bash
set -euo pipefail

# A rule refuses when it cannot work out what to check; it never passes on a failed lookup (#204).
# The CI path, no agent turn: `sr-checks run` judges a committed range with the project's rules. A lookup of
# the rule is made to FAIL by a shim of the tool it uses (jq, git, grep, sort, awk, cp or mktemp) put first on
# PATH for that run only. A shim fails when the joined arguments contain $FAIL_<TOOL>_ARGS and, when set:
#   FAIL_IN     the script running it (its command line, read from the shim's parent) contains this: a
#               lookup both subjects.sh and prepare.sh make is failed in one of them only
#   FAIL_NTH    only the Nth such call of the run (a lookup a script makes twice)
#   FAIL_STDIN  only when the tool's stdin contains this (an exact program shared by two lookups)
#   FAIL_RC     the exit status (default 3); FAIL_OUT: print this and exit 0 instead (a lookup that answers wrongly)
# and a mktemp shim makes $PRE_MKTEMP_DIR inside the directory it creates when its arguments contain
# $PRE_MKTEMP_ARGS: a work file of that name then cannot be written (a directory is in its place; no
# privileges matter, as they would for a read-only directory). The rule must then refuse, saying which
# lookup or write failed. A control run, with nothing failing, shows the same range is otherwise judged.
# The verdicts are kept by content, so every injection is its own commit (a stored verdict is replayed).
YQ="$(PATH="/opt/homebrew/bin:/usr/local/bin:/usr/bin" command -v yq)" || { echo "needs yq on the machine running the case" >&2; exit 1; }
mkdir -p "$TMPDIR/tools" "$TMPDIR/shim" "$TMPDIR/count" && ln -sf "$YQ" "$TMPDIR/tools/yq" && export PATH="$PATH:$TMPDIR/tools"
for tool in jq git grep sort awk cp mktemp; do
  up="$(printf '%s' "$tool" | tr a-z A-Z)"; real="$(command -v "$tool")"
  cat >"$TMPDIR/shim/$tool" <<SHIM
#!/usr/bin/env bash
hit=0
if [ -n "\${FAIL_${up}_ARGS:-}" ]; then case "\$*" in *"\$FAIL_${up}_ARGS"*) hit=1 ;; esac; fi
if [ "\$hit" = 1 ] && [ -n "\${FAIL_IN:-}" ]; then case "\$(ps -o args= -p \$PPID 2>/dev/null)" in *"\$FAIL_IN"*) ;; *) hit=0 ;; esac; fi
if [ "\$hit" = 1 ] && [ -n "\${FAIL_STDIN:-}" ]; then
  in="\$(cat)"; case "\$in" in *"\$FAIL_STDIN"*) ;; *) hit=0 ;; esac
  if [ "\$hit" = 0 ]; then printf '%s' "\$in" | "$real" "\$@"; exit \$?; fi
fi
if [ "\$hit" = 1 ] && [ -n "\${FAIL_NTH:-}" ]; then
  k=\$(( \$(cat "$TMPDIR/count/$tool" 2>/dev/null || echo 0) + 1 )); echo \$k >"$TMPDIR/count/$tool"
  [ "\$k" = "\$FAIL_NTH" ] || hit=0
fi
if [ "\$hit" = 1 ]; then
  [ -n "\${FAIL_OUT:-}" ] && { printf '%s' "\$FAIL_OUT"; exit 0; }
  exit "\${FAIL_RC:-3}"
fi
if [ "$tool" = mktemp ] && [ -n "\${PRE_MKTEMP_ARGS:-}" ]; then case "\$*" in *"\$PRE_MKTEMP_ARGS"*)
  d="\$("$real" "\$@")" || exit \$?; mkdir "\$d/\$PRE_MKTEMP_DIR" || exit 1; printf '%s\n' "\$d"; exit 0 ;; esac; fi
exec "$real" "\$@"
SHIM
  chmod +x "$TMPDIR/shim/$tool"
done
git init -q .
c() { git add -A && git -c user.name=t -c user.email=t@t commit -q -m "$1"; }
# run_rule — judge BASE..HEAD with the FAIL_*/PRE_* variables of the call in force; the rule's events land in $SR_EVENTS_FILE
run_rule() {
  : > "$SR_EVENTS_FILE"; rm -f "$TMPDIR"/count/*
  PATH="$TMPDIR/shim:$PATH" sr-checks run --base "$BASE" --head HEAD >/dev/null 2>&1 && ran=0 || ran=$?
}
expect_passed() {   # LABEL
  jq -es '[.[] | select(.kind=="FileGuardChecked" and .rule=="module-distinct")] | length > 0 and all(.[]; .outcome=="passed")' "$SR_EVENTS_FILE" >/dev/null ||
    { jq -c . "$SR_EVENTS_FILE" >&2; echo "$1: $RULE did not pass (sr-checks exit $ran)" >&2; exit 1; }
}
expect_refused() {   # LABEL SUBSTRING : refused, and the reason says it
  jq -es --arg s "$2" 'any(.[]; .kind=="FileGuardChecked" and .rule=="module-distinct" and .outcome=="refused" and (.reason|contains($s)))' "$SR_EVENTS_FILE" >/dev/null ||
    { jq -c . "$SR_EVENTS_FILE" >&2; echo "$1: $RULE did not refuse with a reason saying '$2' (sr-checks exit $ran)" >&2; exit 1; }
}
# inject LABEL REASON — a fresh commit on BASE (change_for N makes it), with the failure the caller set in force: refused with REASON
n=0
inject() {
  n=$((n + 1)); git checkout -q -b "inject$n" "$BASE"; change_for "$n"; c "change $n"
  run_rule; expect_refused "$1" "$2"
}
RULE=module-distinct
# (expect_passed and expect_refused above select the events of this rule by its literal name, as rule-tests-rigorous requires)
# the judge is a mock that passes: what is under test is what feeds it (prepare.sh; subjects.sh runs first and
# makes some of the same lookups, so each failure here is limited to prepare.sh by FAIL_IN)
export SR_CHECKS_JUDGE_MOCKS='{"file-guard/module-distinct/distinct-concern":"'"$SR_TEST_CASE_DIR"'/judge.sh"}'
mkdir -p internal/a internal/b
# a's second glob matches nothing: grep -c exits 1 for a count of 0, which is a count and no failure
change_for() { printf 'concern: "a owns a (%s)"\nhome: ["internal/a/**", "internal/a/none/**"]\napi: ["internal/a"]\n' "$1" >internal/a/module.yaml; }
change_for base
printf 'concern: "b owns b"\nhome: ["internal/b/**"]\napi: ["internal/b"]\n' >internal/b/module.yaml
printf 'package a\n' >internal/a/a.go; printf 'package b\n' >internal/b/b.go
c base; BASE=$(git rev-parse HEAD)

# control: a module.yaml changes and nothing fails: judged (by the mock) and passed, a glob with no files included
git checkout -q -b control "$BASE"; change_for 0; c control
run_rule; expect_passed "control"

FAIL_IN=prepare.sh FAIL_JQ_ARGS='select(.dir == $d)' \
  inject "the module lookup" "could not look up the module internal/a"
FAIL_IN=prepare.sh FAIL_GIT_ARGS='ls-files -s' \
  inject "the home listing" "the files the home globs of internal/a match could not be listed"
FAIL_IN=prepare.sh FAIL_JQ_ARGS='.home[]' FAIL_NTH=2 \
  inject "the home globs" "could not read the home globs of internal/a"
FAIL_IN=prepare.sh FAIL_SORT_ARGS='-u' \
  inject "a glob's files" "could not list the files the home glob internal/a/** matches"
FAIL_IN=prepare.sh FAIL_GREP_ARGS='-c .' FAIL_RC=2 \
  inject "a glob's file count (grep's own failure, status 2)" "could not count the files the home glob internal/a/** matches"
FAIL_IN=prepare.sh FAIL_JQ_ARGS='{glob: $g' \
  inject "a glob's record" "could not record the files the home glob internal/a/** matches"

# the unsplit rule: prepare.sh run with no subject, which the engine never does for a rule that has subjects:
# (so it is run directly, as the engine runs it, with the rule's own payload from `sr-checks changeset`
# minus the subject): the first module.yaml of the changeset is the module, and failing to find it refuses
git checkout -q -b unsplit "$BASE"; change_for unsplit; c unsplit
REPO="$(pwd)"
sr-checks changeset --rule module-distinct --base "$BASE" --head HEAD | jq -c '[.payload // .subjects[].payload][0] | del(.subject)' >"$TMPDIR/unsplit.json"
unsplit() {
  (cd "$SR_TEST_SLOPRAIL_DIR/file-guard/module-distinct" &&
    SR_TREE="$REPO" SR_GUARDRAIL_DIR="$PWD" PATH="$TMPDIR/shim:$PATH" ./prepare.sh <"$TMPDIR/unsplit.json")
}
out="$(unsplit)" && rc=0 || rc=$?
jq -e '.additionalContext.module.dir == "internal/a"' <<<"$out" >/dev/null || { echo "unsplit control: prepare.sh did not name internal/a: $out" >&2; exit 1; }
out="$(FAIL_JQ_ARGS='rtrimstr("/module.yaml")' unsplit)" && rc=0 || rc=$?
jq -e '.reason | contains("the changed module.yaml could not be worked out")' <<<"$out" >/dev/null || { echo "unsplit: no refusal for the module.yaml of the changeset: $out" >&2; exit 1; }
