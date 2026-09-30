#!/usr/bin/env bash
# Proves each deterministic part of the proposed rules refuses what it should
# and passes what it should, by calling it the way the engine will: a Changeset
# payload on stdin, SR_TREE pointing at a committed tree, SR_GUARDRAIL_DIR at the
# rule's folder. The judges (.md.j2) are not exercised here: they need the
# engine. Their subjects scripts are, since those decide what a judge sees.
#
#   proposals/commit-based-guardrails/tests/run.sh
set -uo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
rules="$here/../rules"
fg="$rules/file-guard"
work="$(mktemp -d "${TMPDIR:-/tmp}/cbg-tests.XXXXXX")"
trap 'rm -rf "$work"' EXIT

pass=0 fail=0
ok()  { pass=$((pass + 1)); printf '  ok    %s\n' "$1"; }
bad() { fail=$((fail + 1)); printf '  FAIL  %s\n' "$1"; [ -n "${2:-}" ] && printf '        %s\n' "$2"; }

# run RULE SCRIPT PAYLOAD — runs a rule's script as the engine would; sets $out, $rc.
run() {
  out="$(cd "$fg/$1" && printf '%s' "$3" | SR_GUARDRAIL_DIR="$fg/$1" SR_GUARDRAIL="$1" SR_TREE="$tree" "./$2" 2>&1)"
  rc=$?
}
# expect NAME WANT(pass|refuse) [SUBSTRING]
expect() {
  local name="$1" want="$2" sub="${3:-}"
  if [ "$want" = pass ] && [ "$rc" -eq 0 ]; then ok "$name"
  elif [ "$want" = refuse ] && [ "$rc" -ne 0 ] && { [ -z "$sub" ] || printf '%s' "$out" | grep -Fq -- "$sub"; }; then ok "$name"
  else bad "$name" "want $want${sub:+ ($sub)}, got rc=$rc: $(printf '%s' "$out" | head -c 400)"; fi
}

# changeset FILES_JSON [CITATIONS_JSON] — a Changeset payload.
changeset() {
  jq -n -c --argjson f "$1" --argjson c "${2:-[]}" \
    '{event: {kind: "Changeset"}, changeset: {base: "b", head: "h", commits: [], files: $f, others: [], citations: $c}}'
}
# file PATH STATUS OLD NEW [DIFF] — one changeset file entry.
file() {
  jq -n -c --arg p "$1" --arg s "$2" --arg o "$3" --arg n "$4" --arg d "${5:-}" \
    '{path: $p, status: $s, oldPath: "", oldContent: $o, newContent: $n, oldMarkers: [], newMarkers: [], diff: $d}'
}
nlines() { local i; for ((i = 1; i <= $1; i++)); do echo "// line $i"; done; }

newtree() {
  tree="$work/$1"
  mkdir -p "$tree" && git -C "$tree" init -q && git -C "$tree" config user.email t@t && git -C "$tree" config user.name t
}
commit() { git -C "$tree" add -A && git -C "$tree" -c commit.gpgsign=false commit -q -m "${1:-fixture}" --allow-empty; }

# ---------------------------------------------------------------------------
echo "matrix-covered/coverage.sh"
newtree cov
mkdir -p "$tree/spec" "$tree/x-mock/internal" "$tree/x-mock/e2e"
cat >"$tree/spec/invariants.yaml" <<'EOF'
invariants:
  - {id: inv.a, statement: A holds}
  - {id: inv.b, statement: B holds}
EOF
cat >"$tree/spec/matrix.yaml" <<'EOF'
items:
  - {id: m.a, covers: [inv.a], case: {x: 1}, expect: A}
  - {id: m.b, covers: [inv.b], case: {x: 2}, expect: B}
EOF
printf 'package internal\n// sr:invariant inv.a\nfunc A() {}\n\n  // sr:invariant "inv.b"\nfunc B() {}\n' >"$tree/x-mock/internal/a.go"
printf 'package e2e\n// sr:proves m.a\nfunc TestA() {}\n# sr:proves m.b\nfunc TestB() {}\n' >"$tree/x-mock/e2e/a_test.go"
commit
p="$(changeset "[$(file spec/matrix.yaml M '' '')]")"
run matrix-covered coverage.sh "$p"; expect "complete matrix passes" pass

sed -i '' '/sr:proves m.b/d' "$tree/x-mock/e2e/a_test.go"; commit
run matrix-covered coverage.sh "$p"; expect "item without a test is refused" refuse "matrix item 'm.b' has no test"

printf '// sr:proves m.b\n// sr:proves m.zzz\n' >>"$tree/x-mock/e2e/a_test.go"; commit
run matrix-covered coverage.sh "$p"; expect "proves naming no item is refused" refuse "sr:proves 'm.zzz' names no matrix item"

sed -i '' '/m.zzz/d' "$tree/x-mock/e2e/a_test.go"
printf '  - {id: inv.c, statement: C holds}\n' >>"$tree/spec/invariants.yaml"; commit
run matrix-covered coverage.sh "$p"; expect "uncovered invariant is refused" refuse "invariant 'inv.c' is covered by no matrix item"
expect "unimplemented invariant is refused" refuse "invariant 'inv.c' has no implementation"

git -C "$tree" checkout -q -- . 2>/dev/null; git -C "$tree" reset -q --hard HEAD~1
printf '// sr:proves m.a\n' >>"$tree/x-mock/internal/a.go"; commit
run matrix-covered coverage.sh "$p"; expect "proves outside a test is refused" refuse "sr:proves belongs in a *_test.go"

rm "$tree/spec/matrix.yaml"; commit
run matrix-covered coverage.sh "$p"; expect "missing matrix is refused" refuse "spec/matrix.yaml is missing"

tree="$work/nogit"; mkdir -p "$tree/spec"; cp "$work/cov/spec/invariants.yaml" "$tree/spec/"
printf 'items: []\n' >"$tree/spec/matrix.yaml"
run matrix-covered coverage.sh "$p"; expect "a git error fails closed" refuse "could not search the committed tree"
tree="$work/cov"
out="$(cd "$fg/matrix-covered" && printf '%s' "$p" | SR_GUARDRAIL_DIR="$fg/matrix-covered" ./coverage.sh 2>&1)"; rc=$?
expect "no SR_TREE fails closed" refuse "SR_TREE is not set"

# ---------------------------------------------------------------------------
echo "invariant-grounded/subjects.sh · matrix-grounded/subjects.sh"
newtree subj
mkdir -p "$tree/spec"
old_inv=$'invariants:\n  - {id: inv.a, statement: A holds}\n  - {id: inv.b, statement: B holds}\n'
new_inv=$'invariants:\n  - id: inv.a\n    statement: A holds\n  - {id: inv.b, statement: B always holds}\n  - {id: inv.c, statement: C holds}\n'
printf '%s' "$new_inv" >"$tree/spec/invariants.yaml"
printf 'items:\n  - {id: m.b, covers: [inv.b], case: {x: 2}, expect: B}\n' >"$tree/spec/matrix.yaml"
commit
run invariant-grounded subjects.sh "$(changeset "[$(file spec/invariants.yaml M "$old_inv" "$new_inv")]")"
got="$(printf '%s' "$out" | jq -r '[.subjects[] | "\(.id):\(.context.change)"] | join(",")' 2>/dev/null)"
[ "$got" = "inv.b:changed,inv.c:added" ] && ok "a reformat is not a change; a rewording and an addition are" ||
  bad "invariant subjects" "got '$got' ($out)"

run matrix-grounded subjects.sh "$(changeset "[$(file spec/matrix.yaml A '' "$(cat "$tree/spec/matrix.yaml")")]")"
got="$(printf '%s' "$out" | jq -r '.subjects[0] | "\(.id):\(.context.change):\(.context.invariants[0].statement)"' 2>/dev/null)"
[ "$got" = "m.b:added:B always holds" ] && ok "a matrix item's subject carries the statements it covers" ||
  bad "matrix subjects" "got '$got' ($out)"

# ---------------------------------------------------------------------------
echo "test-matches-item/subjects.sh"
tree="$work/cov"; git -C "$tree" reset -q --hard HEAD~1
f="$(file x-mock/e2e/a_test.go M '' '' | jq -c '.newMarkers = [{kind: "proves", fqn: "m.a", line: 2}]')"
run test-matches-item subjects.sh "$(changeset "[$f]")"
got="$(printf '%s' "$out" | jq -r '.subjects[] | "\(.id):\(.context.tests[0].path):\(.context.tests[0].text | test("func TestA"))"' 2>/dev/null)"
[ "$got" = "m.a:x-mock/e2e/a_test.go:true" ] && ok "a changed proving test yields its item, with every proving test's text" ||
  bad "test-matches-item subjects" "got '$got' ($out)"

# ---------------------------------------------------------------------------
echo "adr-0001-file-size/size.sh"
tree="$work/cov"
run adr-0001-file-size size.sh "$(changeset "[$(file x-mock/internal/new.go A '' "$(nlines 150)")]")"
expect "a new 150-line file passes" pass
run adr-0001-file-size size.sh "$(changeset "[$(file x-mock/internal/new.go A '' "$(nlines 151)")]")"
expect "a new 151-line file is refused" refuse "over ADR-0001's limit of 150"
run adr-0001-file-size size.sh "$(changeset "[$(file x-mock/e2e/new_test.go A '' "$(nlines 400)")]")"
expect "a 400-line test passes" pass
run adr-0001-file-size size.sh "$(changeset "[$(file claude-mock/internal/runner/session.go M "$(nlines 701)" "$(nlines 702)")]")"
expect "an exception that grows is refused" refuse "may not grow (701 → 702 lines)"
run adr-0001-file-size size.sh "$(changeset "[$(file claude-mock/internal/runner/session.go M "$(nlines 701)" "$(nlines 650)")]")"
expect "an exception that shrinks passes" pass
run adr-0001-file-size size.sh "$(jq -n -c --arg n "$(nlines 151)" '{event: {kind: "PreFileCreate", path: "a/b.go", newContent: $n, resultKnown: true}}')"
expect "gate: a Write of 151 lines is refused before it lands" refuse "over ADR-0001's limit"
run adr-0001-file-size size.sh "$(jq -n -c '{event: {kind: "PreFileUpdate", path: "a/b.go", oldContent: "x", newContent: "", resultKnown: false}}')"
expect "gate: an unpredictable edit is left to the commit check" pass

# ---------------------------------------------------------------------------
echo "adr-0002-layering/imports.sh"
newtree layer
mkdir -p "$tree/core/c" "$tree/a-mock/x" "$tree/b-mock/y"
printf 'module example.com/m\n\ngo 1.22\n' >"$tree/go.mod"
printf 'package c\n' >"$tree/core/c/c.go"
printf 'package y\n' >"$tree/b-mock/y/y.go"
printf 'package x\n\nimport _ "example.com/m/core/c"\n' >"$tree/a-mock/x/x.go"
commit
run adr-0002-layering imports.sh "$(changeset "[$(file a-mock/x/x.go M '' '')]")"
expect "a mock importing core passes" pass
printf 'package x\n\nimport _ "example.com/m/b-mock/y"\n' >"$tree/a-mock/x/x.go"; commit
run adr-0002-layering imports.sh "$(changeset "[$(file a-mock/x/x.go M '' '')]")"
expect "a mock importing another mock is refused" refuse "(another mock, b-mock)"
printf 'package x\n' >"$tree/a-mock/x/x.go"
printf 'package c\n\nimport _ "example.com/m/a-mock/x"\n' >"$tree/core/c/c.go"; commit
run adr-0002-layering imports.sh "$(changeset "[$(file core/c/c.go M '' '')]")"
expect "core importing a mock is refused" refuse "(core) imports example.com/m/a-mock/x"

# ---------------------------------------------------------------------------
echo "adr-0003-subprocess-env/no-own-env.sh"
run adr-0003-subprocess-env no-own-env.sh "$(changeset "[$(file claude-mock/internal/runner/newproc.go A '' $'package runner\n\tcmd.Env = append(os.Environ(), "X=1")')]")"
expect "a new site assigning cmd.Env is refused" refuse "claude-mock/internal/runner/newproc.go assigns cmd.Env"
run adr-0003-subprocess-env no-own-env.sh "$(changeset "[$(file core/procenv/env.go A '' $'package procenv\n\tcmd.Env = env')]")"
expect "core/procenv may assign it" pass
run adr-0003-subprocess-env no-own-env.sh "$(changeset "[$(file claude-mock/internal/runner/runner.go M $'\tcmd.Env = buildEnv(cfg, tr)' $'\tcmd.Env = buildEnv(cfg, tr)\n\tx := 1')]")"
expect "a legacy site that keeps its one assignment passes" pass
run adr-0003-subprocess-env no-own-env.sh "$(changeset "[$(file claude-mock/internal/runner/runner.go M $'\tcmd.Env = buildEnv(cfg, tr)' $'\tcmd.Env = buildEnv(cfg, tr)\n\tc2.Env = other()')]")"
expect "a legacy site that adds one is refused" refuse "adds a cmd.Env assignment (1 → 2)"
run adr-0003-subprocess-env no-own-env.sh "$(changeset "[$(file claude-mock/internal/runner/newproc.go A '' $'\tif cmd.Env == nil {}')]")"
expect "a comparison is not an assignment" pass

# ---------------------------------------------------------------------------
echo "adr-0004-capability-once/one-site-per-capability.sh"
newtree caps
mkdir -p "$tree/core/stop" "$tree/claude-mock/adapter" "$tree/cursor-mock/adapter"
printf 'package stop\n// sr:capability stop.block\n' >"$tree/core/stop/stop.go"
printf 'package adapter\n// sr:provides stop.block claude\n' >"$tree/claude-mock/adapter/stop.go"
commit
p="$(changeset "[$(file core/stop/stop.go M '' '')]")"
run adr-0004-capability-once one-site-per-capability.sh "$p"; expect "one core site and a matching adapter pass" pass
printf 'package adapter\n// sr:capability stop.block\n' >"$tree/cursor-mock/adapter/stop.go"; commit
run adr-0004-capability-once one-site-per-capability.sh "$p"; expect "a second implementation is refused" refuse "implemented in more than one place"
expect "an implementation outside core is refused" refuse "declares capability 'stop.block' outside core/"
printf 'package adapter\n// sr:provides stop.block claude\n' >"$tree/cursor-mock/adapter/stop.go"; commit
run adr-0004-capability-once one-site-per-capability.sh "$p"; expect "provides under the wrong harness is refused" refuse "must sit under claude-mock/"
printf 'package adapter\n// sr:provides compact cursor\n' >"$tree/cursor-mock/adapter/stop.go"; commit
run adr-0004-capability-once one-site-per-capability.sh "$p"; expect "provides of an undeclared capability is refused" refuse "provides 'compact', which no core/ code declares"

# ---------------------------------------------------------------------------
echo "adr-grounded/needs-words.sh · subjects.sh"
adr=.sloprail/file-guard/adr-0001-file-size/ADR.md
run adr-grounded needs-words.sh "$(changeset "[$(file "$adr" M x y $'--- a\n+++ b\n@@\n-  - claude-mock/run.go   # 238\n')]")"
[ "$rc" -eq 1 ] && ok "shrinking an exception list needs no words (waived)" || bad "needs-words shrink" "rc=$rc $out"
run adr-grounded needs-words.sh "$(changeset "[$(file "$adr" M x y $'--- a\n+++ b\n@@\n+  - core/big.go\n')]")"
[ "$rc" -eq 0 ] && ok "adding an exception needs words" || bad "needs-words add" "rc=$rc $out"
run adr-grounded needs-words.sh "$(changeset "[$(file .sloprail/file-guard/adr-0009-x/ADR.md A '' y $'+new')]")"
[ "$rc" -eq 0 ] && ok "a new ADR needs words" || bad "needs-words new" "rc=$rc $out"
run adr-grounded subjects.sh "$(changeset "[$(file "$adr" M old new d1), $(file .sloprail/file-guard/adr-0001-file-size/size.sh M a b d2), $(file .sloprail/file-guard/adr-0003-subprocess-env/ADR.md A '' n3 d3)]")"
got="$(printf '%s' "$out" | jq -r '[.subjects[] | "\(.id)=\(.files | length):\(.context.adr_after)"] | join(",")' 2>/dev/null)"
[ "$got" = "adr-0001-file-size=2:new,adr-0003-subprocess-env=1:n3" ] && ok "one subject per ADR folder touched" ||
  bad "adr-grounded subjects" "got '$got' ($out)"

# ---------------------------------------------------------------------------
echo "adr-undeclared/all-adrs.sh"
newtree adrs
mkdir -p "$tree/.sloprail/file-guard/adr-0001-a" "$tree/.sloprail/file-guard/adr-0002-b"
echo "# A" >"$tree/.sloprail/file-guard/adr-0001-a/ADR.md"; echo "# B" >"$tree/.sloprail/file-guard/adr-0002-b/ADR.md"; commit
run adr-undeclared all-adrs.sh "$(changeset '[]')"
got="$(printf '%s' "$out" | jq -r '[.additionalContext.adrs[].id] | join(",")' 2>/dev/null)"
[ "$got" = "adr-0001-a,adr-0002-b" ] && ok "the classifier is handed every ADR in the committed tree" || bad "all-adrs" "got '$got' ($out)"

echo
echo "$pass passed, $fail failed"
[ "$fail" -eq 0 ]
