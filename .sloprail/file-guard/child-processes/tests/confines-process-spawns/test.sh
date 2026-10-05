#!/usr/bin/env bash
set -euo pipefail

# child-processes: only internal/procexec starts a child process or sets its environment; the legacy files an ADR
# lists under `exceptions` may keep their sites but the total outside procexec may not grow. The CI path, no agent
# turn: `sr-checks run` judges committed ranges with the project's rules (only this rule runs); its outcome is
# asserted on the FileGuardChecked events, one scenario per commit (a verdict is cached by content):
#   a spawn outside procexec                     -> refused, "use internal/procexec", naming the file
#   the spawn moved into procexec (recovery)     -> passed
#   a spawn in a test, in procexec, a non-Go file -> passed / not judged (the boundary)
#   a legacy exception left alone                -> passed; a second site added to it -> refused (the ratchet)
export GIT_AUTHOR_NAME=t GIT_AUTHOR_EMAIL=t@t GIT_COMMITTER_NAME=t GIT_COMMITTER_EMAIL=t@t
YQ="$(PATH="/opt/homebrew/bin:/usr/local/bin:/usr/bin" command -v yq)" || { echo "needs yq on the machine running the case" >&2; exit 1; }
mkdir -p "$TMPDIR/tools" && ln -sf "$YQ" "$TMPDIR/tools/yq" && export PATH="$PATH:$TMPDIR/tools"
RULE=child-processes
git init -q -b main .
cp -R "$SR_TEST_SLOPRAIL_DIR" .sloprail
rm -rf .sloprail/tests .sloprail/file-guard/*/tests .sloprail/gate/*/tests .sloprail/file-guard/structure.tests
# only this rule runs: the others are switched off for the case
{ echo 'disabled:'; echo '  - sloprail/gate/ci-verify-required'; echo '  - sloprail/file-guard/rule-tests-pass'
  for d in .sloprail/file-guard/*/ .sloprail/gate/*/; do n="$(basename "$(dirname "$d")")/$(basename "$d")"; [ "$n" = "file-guard/$RULE" ] || echo "  - $n"; done; } >.sloprail/config.yaml
c() { git add -A && git commit -q -m "$1"; }
run_rule() {
  : > "$SR_EVENTS_FILE"
  sr-checks run --base "$BASE" --head HEAD >/dev/null 2>&1 && ran=0 || ran=$?
}
expect_passed() {   # LABEL
  jq -es '[.[] | select(.kind=="FileGuardChecked" and .rule=="child-processes")] | length > 0 and all(.[]; .outcome=="passed")' "$SR_EVENTS_FILE" >/dev/null ||
    { jq -c . "$SR_EVENTS_FILE" >&2; echo "$1: $RULE did not pass (sr-checks exit $ran)" >&2; exit 1; }
}
expect_refused() {   # LABEL SUBSTRING...
  local label="$1" s; shift
  for s in "$@"; do
    jq -es --arg s "$s" 'any(.[]; .kind=="FileGuardChecked" and .rule=="child-processes" and .outcome=="refused" and (.reason|contains($s)))' "$SR_EVENTS_FILE" >/dev/null ||
      { jq -c . "$SR_EVENTS_FILE" >&2; echo "$label: $RULE did not refuse with a reason saying '$s' (sr-checks exit $ran)" >&2; exit 1; }
  done
}
expect_not_judged() {   # LABEL
  jq -es '[.[] | select(.kind=="FileGuardChecked" and .rule=="child-processes")] | length == 0' "$SR_EVENTS_FILE" >/dev/null ||
    { jq -c . "$SR_EVENTS_FILE" >&2; echo "$1: $RULE was asked about a change outside its match" >&2; exit 1; }
}

mkdir -p adr/procs internal/procexec internal/other internal/legacy
printf -- '---\nconcern: processes start in one place\nsloprails: [file-guard/child-processes]\nexceptions:\n  - internal/legacy/l.go\n---\n## Concern\nc\n## Decision\n- only internal/procexec starts a process\n' >adr/procs/ADR.md
printf 'package procexec\n\nfunc Run() {}\n' >internal/procexec/p.go
printf 'package other\n\nfunc f() {}\n' >internal/other/o.go
printf 'package legacy\n\nfunc l() { exec.Command("ls") }\n' >internal/legacy/l.go
c base
BASE=$(git rev-parse HEAD)

# a spawn outside procexec: refused with the rule's reason, naming the file
git checkout -q -b spawn "$BASE"
printf 'package other\n\nfunc f() { exec.Command("ls") }\n' >internal/other/o.go; c "o spawns"
run_rule
expect_refused "a spawn outside procexec" "internal/other/o.go starts a process or sets its environment: use internal/procexec"
# recovery: the spawn moves into procexec, and the same range passes
printf 'package other\n\nfunc f() {}\n' >internal/other/o.go
printf 'package procexec\n\nfunc Run() { exec.Command("ls") }\n' >internal/procexec/p.go; c "the spawn moves into procexec"
run_rule
expect_passed "the spawn moved into procexec"

# setting a process's environment is a spawn too
git checkout -q -b env "$BASE"
printf 'package other\n\nfunc f() { cmd.Env = nil }\n' >internal/other/o.go; c "o sets an environment"
run_rule
expect_refused "an environment set outside procexec" "internal/other/o.go starts a process or sets its environment"

# the boundary: a test file may spawn, and a non-Go file is not judged at all
git checkout -q -b edge "$BASE"
printf 'package other\n\nfunc TestF() { exec.Command("ls") }\n' >internal/other/o_test.go; c "a test spawns"
run_rule
expect_passed "a spawn in a test file"
git checkout -q -b notgo "$BASE"
printf 'exec.Command("ls")\n' >internal/other/notes.txt; c "a note"
run_rule
expect_not_judged "a non-Go file"

# a legacy exception: touched without a new site passes; a second site in it is refused (the ratchet)
git checkout -q -b legacy "$BASE"
printf 'package legacy\n\n// kept\nfunc l() { exec.Command("ls") }\n' >internal/legacy/l.go; c "legacy touched"
run_rule
expect_passed "a legacy exception edited without a new site"
printf 'package legacy\n\nfunc l() { exec.Command("ls"); exec.Command("pwd") }\nfunc m() {\n\texec.Command("id")\n}\n' >internal/legacy/l.go; c "legacy grows"
run_rule
expect_refused "a legacy exception that grows" "the code outside the allowed place now holds 2 sites, up from 1: legacy sites may only be removed"
