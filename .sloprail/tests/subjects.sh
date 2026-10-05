#!/usr/bin/env bash
# Rule tests for the `subjects:` scripts: a change to one unit leaves another unit's subject key
# alone, and a change to what one unit's verdict depends on (a file it references but the range
# does not select) changes THAT unit's key.
#
# A subject's key is its files' content plus its fingerprint (sloprail's guardKey), so this builds
# a small repository, copies this checkout's .sloprail into it, asks the engine what each rule's
# subjects are over a range (`sr-checks changeset --rule`, which runs the `subjects:` script and
# runs no check and no judge) and compares those keys across ranges.
#
#   .sloprail/tests/subjects.sh          needs sr-checks (SR_CHECKS=path), git, jq, yq, shasum
#
# A judge cannot be run here: it asks a model. What is covered for the judged rules is what feeds
# the model, the split and its keys, and each prepare.sh's output for the subject it is handed.
set -uo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
SR="${SR_CHECKS:-sr-checks}"
command -v "$SR" >/dev/null && command -v jq >/dev/null && command -v yq >/dev/null || { echo "needs sr-checks, jq and yq" >&2; exit 2; }
T="$(mktemp -d "${TMPDIR:-/tmp}/sr-subjects-test.XXXXXX")"; trap 'rm -rf "$T"' EXIT
export GIT_AUTHOR_NAME=t GIT_AUTHOR_EMAIL=t@t GIT_COMMITTER_NAME=t GIT_COMMITTER_EMAIL=t@t GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_SYSTEM=/dev/null
PASS=0; FAIL=0
ok() { PASS=$((PASS + 1)); }
bad() { FAIL=$((FAIL + 1)); echo "FAIL: $1" >&2; }
eq() { [ "$2" = "$3" ] && ok || bad "$1: '$2' != '$3'"; }
ne() { [ "$2" != "$3" ] && ok || bad "$1: both '$2'"; }

R="$T/repo"; mkdir -p "$R"; cd "$R"
git init -q -b main .
cp -R "$ROOT/.sloprail" .sloprail; rm -rf .sloprail/tests
w() { mkdir -p "$(dirname "$1")"; cat >"$1"; }   # w PATH < content

cap() {   # cap ID RUN [STATEMENT] — a capability one harness provides, citing one run and one doc
  w "spec/capabilities/$1.yaml" <<EOF
statement: ${3:-"$1 works"}
providers:
  claude:
    docs: [https://d.example/$1#s]
    runs: [claude-mock/snapshots/runs/$2]
  codex: pending
EOF
}
run() { w "claude-mock/snapshots/runs/$1/run.yaml" <<<"version: 1"; w "claude-mock/snapshots/runs/$1/samples/20240101-000000/events.jsonl" <<<"$2"; }

# the base: capabilities a and b, invariants i1 and i2, modules m1 and m2, ADRs one and two
w claude-mock/snapshots/MANIFEST.yaml <<<"pin: 1"
w codex-mock/snapshots/MANIFEST.yaml <<<"pin: 1"
w codex-mock/snapshots/runs/c1/run.yaml <<<"version: 1"
cap a ra; cap b rb; run ra '{"e":1}'; run rb '{"e":1}'
w spec/invariants/i1.yaml <<<"statement: one"; w spec/invariants/i2.yaml <<<"statement: two"
w claude-mock/e2e/a_test.go <<<'// sr:proves a/claude'
w claude-mock/e2e/b_test.go <<<'// sr:proves b/claude'
w claude-mock/e2e/i1_test.go <<<'// sr:proves i1'
w claude-mock/e2e/i2_test.go <<<'// sr:proves i2'
for m in m1 m2; do
  w "internal/$m/module.yaml" <<<"{concern: \"$m owns $m\", home: [\"internal/$m/**\"], api: [\"internal/$m\"]}"
  w "internal/$m/candidates.sh" <<<"git grep -n -E '$(printf %s "$m" | tr a-z A-Z)\\(' -- '*.go' || true"; chmod +x "internal/$m/candidates.sh"
  w "internal/$m/$m.go" <<<"package $m"
done
w internal/other/other.go <<<"package other"
w adr/one/ADR.md <<<$'---\nconcern: c\nsloprails: [file-guard/capability-grounded]\n---\n## Concern\nc\n## Decision\n- d\n'
w adr/two/ADR.md <<<$'---\nconcern: c\nsloprails: [file-guard/invariant-grounded]\n---\n## Concern\nc\n## Decision\n- d\n'
w adr/cover/ADR.md <<<$'---\nconcern: c\nsloprails: [file-guard/module-coverage]\nspace: ["internal/**"]\nexceptions: ["internal/other/**"]\n---\n## Concern\nc\n## Decision\n- d\n'
git add -A && git commit -q -m base && BASE="$(git rev-parse HEAD)"

# edit FILE-WRITING-COMMAND... then commit; prints the new head
step() { git add -A && git commit -q -m "$1" && git rev-parse HEAD; }
back() { git checkout -q "$BASE"; }

# keys RULE HEAD — "<id> <key>" per subject over BASE..HEAD, a key being the subject's files'
# content and its fingerprint, which is all the engine keys it by (besides the rule hash and id)
keys() {
  "$SR" changeset --rule "$1" --base "$BASE" --head "$2" 2>"$T/err" |
    jq -r '.subjects[] | .id as $id | .payload as $p
      | [($p.subject.files // [])[] as $f | ($p.changeset.files[] | select(.path == $f) | [.path, .status, (.newContent // ""), (.oldContent // "")])]
      | "\($id) \([., $p.subject.fingerprint // ""] | tojson | @base64 | gsub("\n"; ""))"' || { cat "$T/err" >&2; return 1; }
}
key() { printf '%s\n' "$1" | awk -v id="$2" '$1 == id {print $2}'; }
ids() { printf '%s\n' "$1" | awk '{print $1}' | sort | tr '\n' ' '; }

# --- capability-grounded: one subject per capability --------------------------------------
back; cap a ra "a changed"; cap b rb "b changed";            H1="$(step ab)"
back; cap a ra "a changed again"; cap b rb "b changed";      H2="$(step a2-b)"
K1="$(keys capability-grounded "$H1")"; K2="$(keys capability-grounded "$H2")"
eq "cap-grounded: one subject per capability" "$(ids "$K1")" "a b "
ne "cap-grounded: a changed, a's key moves" "$(key "$K1" a)" "$(key "$K2" a)"
eq "cap-grounded: a changed, b's key does not" "$(key "$K1" b)" "$(key "$K2" b)"
# a run b cites changes without being selected by the rule: only b's fingerprint can say so
back; cap a ra "a changed"; cap b rb "b changed"; run rb '{"e":2}'; H3="$(step b-run)"
K3="$(keys capability-grounded "$H3")"
ne "cap-grounded: b's cited run changed, b's key moves" "$(key "$K1" b)" "$(key "$K3" b)"
eq "cap-grounded: b's cited run changed, a's key does not" "$(key "$K1" a)" "$(key "$K3" a)"

# --- capability-rigor: one subject per capability; its runs and tests are its own -------------
back; run rb '{"e":2}'; H4="$(step rb)"
back; run ra '{"e":2}'; H5="$(step ra)"
K4="$(keys capability-rigor "$H4")"; K5="$(keys capability-rigor "$H5")"
eq "cap-rigor: a run of b touches b alone" "$(ids "$K4")" "b "
eq "cap-rigor: a run of a touches a alone" "$(ids "$K5")" "a "
back; run rb '{"e":2}'; run ra '{"e":2}'; H6="$(step both-runs)"
K6="$(keys capability-rigor "$H6")"
eq "cap-rigor: both runs, both capabilities" "$(ids "$K6")" "a b "
eq "cap-rigor: b's key is the same whether or not a's run changed" "$(key "$K4" b)" "$(key "$K6" b)"
eq "cap-rigor: a's key is the same whether or not b's run changed" "$(key "$K5" a)" "$(key "$K6" a)"
# a proving test b carries but this range does not select (it is unchanged): declared by the fingerprint
back; cap b rb "b changed"; w claude-mock/e2e/b_test.go <<<$'// sr:proves b/claude\n// edited'; H7="$(step b-test)"
back; cap b rb "b changed"; w claude-mock/e2e/b_test.go <<<$'// sr:proves b/claude\n// edited more'; H8="$(step b-test2)"
K7="$(keys capability-rigor "$H7")"; K8="$(keys capability-rigor "$H8")"
ne "cap-rigor: b's proving test changed, b's key moves" "$(key "$K7" b)" "$(key "$K8" b)"

# --- #87's cell kinds: a pending cell is neither judged nor coverage; an unsupported one (with its docs) is grounded, never rigor
back; w spec/capabilities/u.yaml <<'EOF'
statement: u works
providers:
  claude: {docs: [https://d.example/u#s], runs: [claude-mock/snapshots/runs/ra]}
  codex: {supported: false, reason: not there, docs: [https://c.example/u#s]}
EOF
w codex-mock/snapshots/MANIFEST.yaml <<<"pin: 2"; HU="$(step unsupported-cell)"
eq "cells: an unsupported cell's capability is grounded (its file is new; the codex MANIFEST is no trigger)" "$(ids "$(keys capability-grounded "$HU")")" "u "
eq "cells: ... and only its supported cell is proven (no rigor subject takes codex's MANIFEST)" "$("$SR" changeset --rule capability-rigor --base "$BASE" --head "$HU" | jq -r '[.subjects[].payload.subject.files[] | select(startswith("codex-mock/"))] | length')" "0"

# --- capability-reconciled: one subject per capability a cell or an sr:provides marker change touches ---
back; w claude-mock/internal/a.go <<<$'package internal\n// sr:provides a/claude'; w claude-mock/internal/b.go <<<$'package internal\n// sr:provides b/claude'; BASE_R="$(step adapters)"
RK() { "$SR" changeset --rule capability-reconciled --base "$BASE_R" --head "$1" 2>"$T/err" |
  jq -r '.subjects[] | .id as $id | .payload as $p | "\($id) \([[($p.subject.files // [])[]], $p.subject.fingerprint // ""] | tojson | @base64 | gsub("\n"; ""))"'; }
git checkout -q "$BASE_R"; w claude-mock/internal/a.go <<<$'package internal\n// sr:provides a/claude\n// edited'; HR1="$(step a-adapter-edited)"
git checkout -q "$BASE_R"; cap a ra "a changed"; HR2="$(step a-cell)"
git checkout -q "$BASE_R"; cap a ra "a changed"; cap b rb "b changed"; HR3="$(step ab-cells)"
git checkout -q "$BASE_R"; git rm -q claude-mock/internal/b.go; HR4="$(step b-adapter-gone)"
eq "reconciled: an edit to a's adapter that keeps its marker is no capability's subject (only the unclaimed one, which judges nothing)" "$(ids "$(RK "$HR1")")" "unclaimed "
eq "reconciled: a cell change is that capability's subject alone" "$(ids "$(RK "$HR2")")" "a "
eq "reconciled: two cells, two subjects" "$(ids "$(RK "$HR3")")" "a b "
eq "reconciled: a's key is the same whether or not b's cell changed" "$(key "$(RK "$HR2")" a)" "$(key "$(RK "$HR3")" a)"
eq "reconciled: a removed adapter is its capability's subject" "$(ids "$(RK "$HR4")")" "b "
back

# --- invariant-grounded / invariant-rigor ----------------------------------------------------
back; w spec/invariants/i1.yaml <<<"statement: one!"; w spec/invariants/i2.yaml <<<"statement: two!"; H9="$(step inv)"
back; w spec/invariants/i1.yaml <<<"statement: one?"; w spec/invariants/i2.yaml <<<"statement: two!"; H10="$(step inv2)"
K9="$(keys invariant-grounded "$H9")"; K10="$(keys invariant-grounded "$H10")"
eq "inv-grounded: one subject per invariant" "$(ids "$K9")" "i1 i2 "
ne "inv-grounded: i1 changed, i1's key moves" "$(key "$K9" i1)" "$(key "$K10" i1)"
eq "inv-grounded: i1 changed, i2's key does not" "$(key "$K9" i2)" "$(key "$K10" i2)"
K9="$(keys invariant-rigor "$H9")"; K10="$(keys invariant-rigor "$H10")"
ne "inv-rigor: i1 changed, i1's key moves" "$(key "$K9" i1)" "$(key "$K10" i1)"
eq "inv-rigor: i1 changed, i2's key does not" "$(key "$K9" i2)" "$(key "$K10" i2)"
back; w spec/invariants/i1.yaml <<<"statement: one!"; w claude-mock/e2e/i2_test.go <<<$'// sr:proves i2\n// more'; H11="$(step inv-test)"
K11="$(keys invariant-rigor "$H11")"
eq "inv-rigor: a change to i2's test is i2's and i1's" "$(ids "$K11")" "i1 i2 "
eq "inv-rigor: ... and leaves i1's key as it was" "$(key "$K9" i1)" "$(key "$K11" i1)"

# --- adr-grounded: one subject per ADR ---------------------------------------------------------
back; w adr/one/ADR.md <<<$'---\nconcern: c\nsloprails: [file-guard/capability-grounded]\n---\n## Concern\nc\n## Decision\n- d\n- e\n'
w adr/two/ADR.md <<<$'---\nconcern: c\nsloprails: [file-guard/invariant-grounded]\n---\n## Concern\nc\n## Decision\n- d\n- f\n'; H12="$(step adrs)"
back; w adr/one/ADR.md <<<$'---\nconcern: c\nsloprails: [file-guard/capability-grounded]\n---\n## Concern\nc\n## Decision\n- d\n- e2\n'
w adr/two/ADR.md <<<$'---\nconcern: c\nsloprails: [file-guard/invariant-grounded]\n---\n## Concern\nc\n## Decision\n- d\n- f\n'; H13="$(step adrs2)"
K12="$(keys adr-grounded "$H12")"; K13="$(keys adr-grounded "$H13")"
eq "adr-grounded: one subject per ADR" "$(ids "$K12")" "one two "
ne "adr-grounded: one changed, one's key moves" "$(key "$K12" one)" "$(key "$K13" one)"
eq "adr-grounded: one changed, two's key does not" "$(key "$K12" two)" "$(key "$K13" two)"

# --- adr-matches-sloprails: an ADR and the rules it links ----------------------------------------
back; w .sloprail/file-guard/capability-grounded/note.txt <<<"x"; H14="$(step rule-of-one)"
back; w .sloprail/file-guard/capability-grounded/note.txt <<<"x"; w .sloprail/file-guard/invariant-grounded/note.txt <<<"y"; H15="$(step rules-of-both)"
K14="$(keys adr-matches-sloprails "$H14")"; K15="$(keys adr-matches-sloprails "$H15")"
eq "adr-matches: a rule only one links is one's subject alone" "$(ids "$K14")" "one "
eq "adr-matches: both rules, both ADRs" "$(ids "$K15")" "one two "
eq "adr-matches: two's rule changing leaves one's key alone" "$(key "$K14" one)" "$(key "$K15" one)"
# the ADR's own text is its fingerprint's too: an ADR whose rule file changed in an earlier range
back; w .sloprail/file-guard/capability-grounded/note.txt <<<"x"; w adr/one/ADR.md <<<$'---\nconcern: c\nsloprails: [file-guard/capability-grounded]\n---\n## Concern\nc\n## Decision\n- d\n- new\n'; H16="$(step adr-and-rule)"
ne "adr-matches: one's own text changed, one's key moves" "$(key "$K14" one)" "$(key "$(keys adr-matches-sloprails "$H16")" one)"

# --- module-leaks: one subject per module with candidates ---------------------------------------------
back; w internal/other/other.go <<<$'package other\nfunc x() { M1(1) }'; H17="$(step leak-m1)"
back; w internal/other/other.go <<<$'package other\nfunc x() { M1(1) }'; w internal/other/two.go <<<$'package other\nfunc y() { M2(2) }'; H18="$(step leak-both)"
back; w internal/other/other.go <<<$'package other\nfunc x() { M1(1) }'; w internal/m2/m2.go <<<$'package m2\nfunc y() { M2(3) }'; H19="$(step leak-m1-m2home)"
K17="$(keys module-leaks "$H17")"; K18="$(keys module-leaks "$H18")"; K19="$(keys module-leaks "$H19")"
eq "module-leaks: only m1's search found something" "$(ids "$K17")" "internal/m1 "
eq "module-leaks: both searches found something" "$(ids "$K18")" "internal/m1 internal/m2 "
eq "module-leaks: m2's logic in m2's home is no candidate" "$(ids "$K19")" "internal/m1 "
ne "module-leaks: a new m2 candidate moves m2's key and adds its subject" "$(key "$K17" internal/m2)" "$(key "$K18" internal/m2)"
eq "module-leaks: ... and leaves m1's alone" "$(key "$K17" internal/m1)" "$(key "$K18" internal/m1)"
eq "module-leaks: unrelated m2-home code leaves m1's key alone" "$(key "$K17" internal/m1)" "$(key "$K19" internal/m1)"
back; w internal/other/other.go <<<$'package other\nfunc x() { M1(1) }'; w internal/m1/module.yaml <<<'{concern: "m1 owns more", home: ["internal/m1/**"], api: ["internal/m1"]}'; H20="$(step m1-boundary)"
ne "module-leaks: m1's module.yaml changed, m1's key moves" "$(key "$K17" internal/m1)" "$(key "$(keys module-leaks "$H20")" internal/m1)"

# --- snapshots-current: one subject per harness ------------------------------------------------
back; run ra '{"e":9}'; H21="$(step claude-snap)"
back; w codex-mock/snapshots/runs/c1/run.yaml <<<"version: 2"; H22="$(step codex-snap)"
back; run ra '{"e":9}'; w codex-mock/snapshots/runs/c1/run.yaml <<<"version: 2"; H23="$(step both-snap)"
back; cap a ra "a changed"; H24="$(step cap)"
K21="$(keys snapshots-current "$H21")"; K22="$(keys snapshots-current "$H22")"; K23="$(keys snapshots-current "$H23")"; K24="$(keys snapshots-current "$H24")"
eq "snapshots: claude's snapshots are claude's subject alone" "$(ids "$K21")" "claude "
eq "snapshots: codex's snapshots are codex's subject alone" "$(ids "$K22")" "codex "
eq "snapshots: claude's change leaves codex's key as it was" "$(key "$K22" codex)" "$(key "$K23" codex)"
eq "snapshots: codex's change leaves claude's key as it was" "$(key "$K21" claude)" "$(key "$K23" claude)"
eq "snapshots: a capability change is every harness's" "$(ids "$K24")" "claude codex "

# --- module-distinct: one subject per module.yaml added or changed ---------------------------------
back; w internal/m1/module.yaml <<<'{concern: "m1 owns more", home: ["internal/m1/**"], api: ["internal/m1"]}'; H25="$(step d-m1)"
back; w internal/m1/module.yaml <<<'{concern: "m1 owns more", home: ["internal/m1/**"], api: ["internal/m1"]}'
w internal/m2/module.yaml <<<'{concern: "m2 owns something else", home: ["internal/m2/**"], api: ["internal/m2"]}'; H26="$(step d-m1-m2)"
K25="$(keys module-distinct "$H25")"; K26="$(keys module-distinct "$H26")"
eq "module-distinct: one subject per changed module.yaml" "$(ids "$K26")" "internal/m1 internal/m2 "
ne "module-distinct: another module's concern changed, m1 is judged again" "$(key "$K25" internal/m1)" "$(key "$K26" internal/m1)"
back; w internal/m2/module.yaml <<<'{concern: "m2 owns something else", home: ["internal/m2/**"], api: ["internal/m2"]}'; H27="$(step d-m2)"
K27="$(keys module-distinct "$H27")"
eq "module-distinct: a change to m2 alone is m2's subject alone" "$(ids "$K27")" "internal/m2 "
# a file landing in m1's home is a different surrounding: the home must still fit the concern
back; w internal/m1/module.yaml <<<'{concern: "m1 owns more", home: ["internal/m1/**"], api: ["internal/m1"]}'; w internal/m1/extra.go <<<"package m1"; H28="$(step d-m1-file)"
ne "module-distinct: a file landing in m1's home judges m1 again" "$(key "$K25" internal/m1)" "$(key "$(keys module-distinct "$H28")" internal/m1)"
# the context the judge gets: each rule's prepare.sh, handed one subject, builds that subject alone
prepare() {   # RULE SCRIPT HEAD SUBJECT-ID — prepare's output, over the payload the engine gives that subject
  local p; p="$("$SR" changeset --rule "$1" --base "$BASE" --head "$3" | jq -c --arg id "$4" '.subjects[] | select(.id == $id) | .payload')"
  git checkout -q "$3"
  printf '%s' "$p" | SR_TREE="$R" SR_GUARDRAIL_DIR="$R/.sloprail/file-guard/$1" "$R/.sloprail/file-guard/$1/$2"
}
CTX="$(prepare module-distinct prepare.sh "$H26" internal/m1)"
eq "module-distinct: prepare hands the judge the subject module" "$(jq -r '.additionalContext.module.dir' <<<"$CTX")" "internal/m1"
eq "module-distinct: ... and every other module's concern" "$(jq -r '[.additionalContext.others[].concern] | sort | join("|")' <<<"$CTX")" "m2 owns something else"
HOMES="$(jq -c '.additionalContext.module.homes | map({glob, count})' <<<"$CTX")"
eq "module-distinct: ... and the files each home glob matches" "$HOMES" '[{"glob":"internal/m1/**","count":3}]'
eq "module-distinct: ... listed in a file the judge reads" "$(sort "$(jq -r '.additionalContext.module.homes[0].files' <<<"$CTX")" | tr '\n' ' ')" "internal/m1/candidates.sh internal/m1/m1.go internal/m1/module.yaml "
eq "adr-grounded: prepare builds the subject's ADR alone" "$(prepare adr-grounded prepare.sh "$H12" two | jq -r '[.additionalContext.subjects[].id] | join(",")')" "two"
eq "inv-grounded: prepare builds the subject's invariant alone" "$(prepare invariant-grounded prepare.sh "$H9" i2 | jq -r '[.additionalContext.subjects[].id] | join(",")')" "i2"
eq "inv-rigor: prepare builds the subject's invariant alone" "$(prepare invariant-rigor prepare.sh "$H9" i1 | jq -r '[.additionalContext.subjects[].id] | join(",")')" "i1"
eq "adr-matches: prepare builds the subject's ADR alone" "$(prepare adr-matches-sloprails prepare.sh "$H15" two | jq -r '[.additionalContext.subjects[].id] | join(",")')" "two"
eq "module-leaks: prepare hands the judge the subject's module alone" "$(prepare module-leaks find-leaks.sh "$H18" internal/m2 | jq -r '[.additionalContext.modules[].module] | join(",")')" "internal/m2"

# --- module-coverage: the exceptions only shrink
mc() {   # HEAD — module-coverage's verdict text over BASE..HEAD
  local p; p="$("$SR" changeset --rule module-coverage --base "$BASE" --head "$1" | jq -c '.payload')"
  git checkout -q "$1"
  printf '%s' "$p" | SR_TREE="$R" SR_GUARDRAIL_DIR="$R/.sloprail/file-guard/module-coverage" "$R/.sloprail/file-guard/module-coverage/every-file-mapped.sh"
}
COVER=$'---\nconcern: c\nsloprails: [file-guard/module-coverage]\nspace: ["internal/**"]\nexceptions: %s\n---\n## Concern\nc\n## Decision\n- d\n'
back; w internal/other/new.go <<<"package other"; M1="$(mc "$(step new-under-exception)")"
has() { printf '%s' "$1" | grep -q -- "$2" && echo yes || echo no; }
eq "coverage: a new file under an exception is refused" "$(has "$M1" "added under an exception")" yes
back; w adr/cover/ADR.md <<<"$(printf -- "$COVER" '["internal/other/**", "internal/m1/**"]')"; M2="$(mc "$(step list-grows)")"
eq "coverage: an exceptions list that grows is refused" "$(has "$M2" "exceptions grew")" yes
back; w adr/cover/ADR.md <<<"$(printf -- "$COVER" '["internal/other/**", "internal/m1/**"]')"; git add -A; git commit -q -m grow; BASE2="$(git rev-parse HEAD)"
w adr/cover/ADR.md <<<"$(printf -- "$COVER" '["internal/other/**"]')"; M3="$(BASE="$BASE2" mc "$(step list-shrinks)")"
eq "coverage: an exceptions list that shrinks is not refused for it" "$(has "$M3" "exceptions grew")$(has "$M3" "added under")" nono
back; w internal/other/x/y.go <<<"package x"; w adr/cover/ADR.md <<<"$(printf -- "$COVER" '["internal/other/**", "internal/other/x/**"]')"; M4="$(mc "$(step narrower-glob)")"
eq "coverage: a new glob inside an existing one is a narrowing, not growth" "$(has "$M4" "exceptions grew")" no
back; w adr/cover/ADR.md <<<"$(printf -- "$COVER" '["internal/**"]')"; M5="$(mc "$(step widened-glob)")"
eq "coverage: a widened glob is growth" "$(has "$M5" "exceptions grew")" yes
back; w internal/other/other.go <<<$'package other\n// edited'; M6="$(mc "$(step edit-excepted)")"
eq "coverage: editing an excepted file is fine" "$(has "$M6" "added under")$(has "$M6" "exceptions grew")" nono

# --- docs follow recordings: a doc re-freeze alone re-judges nothing --------------------------------------
# A MANIFEST doc entry changing (a re-freeze), or its pin or fetch date, touches no capability and is in no
# verdict's key: only a recording, the cell file and the code it points at decide a judge. A re-freeze that
# ships WITH a recording changes nothing about the verdict's key beyond the recording.
OLDBASE="$BASE"
back; cap a ra; cap b rb
mani() { w claude-mock/snapshots/MANIFEST.yaml <<<$'pin: "'"$1"$'"\ndocs:\n  https://d.example/a:\n    sha256: '"$2"$'\n    fetched: "'"${4:-2026-10-01}"$'"\n  https://d.example/b:\n    sha256: '"$3"$'\n    fetched: "'"${4:-2026-10-01}"$'"'; }
mani 1 aa bb
w claude-mock/internal/x/x.go <<<$'package x\n\n// sr:provides a/claude\nfunc A() {\n\tone()\n}\n\n// sr:provides b/claude\nfunc B() {\n\ttwo()\n}\n'
BASE="$(step narrow-base)"
back; mani 1 aa2 bb; N1="$(step refreeze-a)"
back; mani 1 aa bb2; N2="$(step refreeze-b)"
back; mani 1 aa2 bb2; N3="$(step refreeze-both)"
back; mani 2 aa bb; N4="$(step binary-bump)"
back; mani 3 aa bb 2026-10-09; N5="$(step bump-and-refreeze-same-sha)"
back; run ra '{"e":3}'; NR1="$(step rerecord-a)"
back; run ra '{"e":3}'; mani 1 aa2 bb; NR2="$(step rerecord-a-and-refreeze-a)"
back; run ra '{"e":3}'; mani 1 aa2 bb2; NR3="$(step rerecord-a-and-refreeze-both)"
for rule in capability-grounded capability-rigor; do
  for n in "$N1" "$N2" "$N3" "$N4" "$N5"; do
    # the rule does not even select a MANIFEST: no file is any subject's
    eq "$rule: a doc-only change (re-freeze, binary bump, same-sha re-freeze) re-judges nothing" "$("$SR" changeset --rule $rule --base "$BASE" --head "$n" | jq -r '[.subjects[] | (.payload.subject.files // [])[]] | length')" "0"
  done
  K1="$(keys $rule "$NR1")"; K2="$(keys $rule "$NR2")"; K3="$(keys $rule "$NR3")"
  eq "$rule: a recording of a's touches a alone" "$(ids "$K1")" "a "
  eq "$rule: ... a re-freeze shipped with it leaves a's key as the recording alone has it" "$(key "$K1" a)" "$(key "$K2" a)"
  eq "$rule: ... and a re-freeze of a page a does not cite changes nothing either" "$(key "$K1" a)" "$(key "$K3" a)"
  eq "$rule: ... and b is not touched by a's recording or the re-freeze" "$(ids "$K3")" "a "
done
# an edited declaration touches the capabilities marked on it, and a file edit outside every declaration none
back; w claude-mock/internal/x/x.go <<<$'package x\n\n// sr:provides a/claude\nfunc A() {\n\tone()\n\tmore()\n}\n\n// sr:provides b/claude\nfunc B() {\n\ttwo()\n}\n'; X1="$(step edit-A)"
back; w claude-mock/internal/x/x.go <<<$'package x\n\n// sr:provides a/claude\nfunc A() {\n\tone()\n}\n\n// sr:provides b/claude\nfunc B() {\n\ttwo()\n\tmore()\n}\n'; X2="$(step edit-B)"
back; w claude-mock/internal/x/x.go <<<$'package x\n\n// sr:provides a/claude\nfunc A() {\n\tone()\n\tmore()\n}\n\n// sr:provides b/claude\nfunc B() {\n\ttwo()\n\tmore()\n}\n'; X3="$(step edit-both)"
back; w claude-mock/internal/x/x.go <<<$'package x\n\nfunc helper() {}\n\n// sr:provides a/claude\nfunc A() {\n\tone()\n}\n\n// sr:provides b/claude\nfunc B() {\n\ttwo()\n}\n'; X4="$(step add-unmarked-func)"
back; w claude-mock/internal/x/x.go <<<$'package x\n\n// sr:provides a/claude\nfunc A() {\n\tone()\n}\n'; X5="$(step drop-B)"
KX1="$(keys capability-rigor "$X1")"; KX2="$(keys capability-rigor "$X2")"; KX3="$(keys capability-rigor "$X3")"
eq "cap-rigor: editing A's declaration touches a alone" "$(ids "$KX1")" "a "
eq "cap-rigor: editing B's declaration touches b alone" "$(ids "$KX2")" "b "
eq "cap-rigor: editing both touches both" "$(ids "$KX3")" "a b "
eq "cap-rigor: an unmarked function added in the file touches no capability" "$(ids "$(keys capability-rigor "$X4")")" "unclaimed "
eq "cap-rigor: removing B's marked declaration touches b (its old marker), not a" "$(ids "$(keys capability-rigor "$X5")")" "b "
# a doc problem is the refusal reason, never an unbound-variable crash: doc_copy's DOC_ERROR must reach
# prepare.sh (it once ran in a $(...) subshell and died with "DOC_ERROR: unbound variable")
back; cap z rz; w spec/capabilities/z.yaml <<<$'statement: z works\nproviders:\n  claude:\n    docs: [https://d.example/unfrozen#s]\n    runs: [claude-mock/snapshots/runs/rz]\n  codex: pending'; run rz '{"e":1}'; w claude-mock/e2e/z_test.go <<<'// sr:proves z/claude'; HZ="$(step unfrozen-doc)"
for rule in capability-grounded capability-rigor; do
  ZP="$("$SR" changeset --rule $rule --base "$BASE" --head "$HZ" | jq -c '.subjects[] | select(.id | startswith("z")) | .payload' | head -n1)"
  git checkout -q "$HZ"
  ZERR="$(printf '%s' "$ZP" | SR_TREE="$R" SR_GUARDRAIL_DIR="$R/.sloprail/file-guard/$rule" "$R/.sloprail/file-guard/$rule/prepare.sh" 2>&1 >/dev/null)"; ZRC=$?
  eq "$rule: a doc the MANIFEST does not freeze refuses (exit 1)" "$ZRC" "1"
  case "$ZERR" in *"no snapshot in claude-mock/snapshots/MANIFEST.yaml freezes https://d.example/unfrozen"*) ok ;; *) bad "$rule: the refusal names the doc problem, got: $ZERR" ;; esac
  case "$ZERR" in *"unbound variable"*) bad "$rule: a doc problem crashed on an unbound variable: $ZERR" ;; *) ok ;; esac
done
BASE="$OLDBASE"

echo "subjects tests: $PASS passed, $FAIL failed"
[ "$FAIL" -eq 0 ]
