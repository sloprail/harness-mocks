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
  jq -es '[.[] | select(.kind=="FileGuardChecked" and .rule=="module-leaks")] | length > 0 and all(.[]; .outcome=="passed")' "$SR_EVENTS_FILE" >/dev/null ||
    { jq -c . "$SR_EVENTS_FILE" >&2; echo "$1: $RULE did not pass (sr-checks exit $ran)" >&2; exit 1; }
}
expect_refused() {   # LABEL SUBSTRING : refused, and the reason says it
  jq -es --arg s "$2" 'any(.[]; .kind=="FileGuardChecked" and .rule=="module-leaks" and .outcome=="refused" and (.reason|contains($s)))' "$SR_EVENTS_FILE" >/dev/null ||
    { jq -c . "$SR_EVENTS_FILE" >&2; echo "$1: $RULE did not refuse with a reason saying '$2' (sr-checks exit $ran)" >&2; exit 1; }
}
# inject LABEL REASON — a fresh commit on BASE (change_for N makes it), with the failure the caller set in force: refused with REASON
n=0
inject() {
  n=$((n + 1)); git checkout -q -b "inject$n" "$BASE"; change_for "$n"; c "change $n"
  run_rule; expect_refused "$1" "$2"
}
RULE=module-leaks
# the judge is a mock that passes: what is under test is what feeds it (subjects.sh, find-leaks.sh, leaks-lib.sh).
# subjects.sh runs first and leaks-lib.sh's setup is the same in both scripts, so a failure there is reported by
# the first one that makes it
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

# leaks-lib.sh: the range's head and its tree, the filtering of a module's candidates
FAIL_JQ_ARGS='.changeset.head' \
  inject "the range's head" "the range's head could not be read"
FAIL_GIT_ARGS='^{tree}' \
  inject "the tree of the range's head" "the tree of the range's head could not be resolved"
FAIL_AWK_ARGS='addedf=' \
  inject "the filtering of a module's candidates" "the candidates of internal/a could not be filtered"
# the one list of the subjects (stdin: with "id"; the module.yaml documents', with "dir", are collected by the same program)
FAIL_JQ_ARGS='-sc .' FAIL_STDIN='"id":' \
  inject "the subjects collected into one list" "the subjects could not be collected into one list"
