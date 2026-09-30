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
# RULE is a file-guard folder name, or gate/<name> for a gate.
run() {
  local dir="$fg/$1"; case "$1" in gate/*) dir="$rules/$1" ;; esac
  out="$(cd "$dir" && printf '%s' "$3" | SR_GUARDRAIL_DIR="$dir" SR_GUARDRAIL="$(basename "$dir")" SR_TREE="$tree" "./$2" 2>&1)"
  rc=$?
}
# run_gate RULE SCRIPT PAYLOAD — a gate runs before the write: no SR_TREE, only the workspace.
run_gate() {
  local dir="$rules/$1"
  out="$(cd "$dir" && printf '%s' "$3" | SR_GUARDRAIL_DIR="$dir" SR_GUARDRAIL="$(basename "$dir")" SR_WORKSPACE="$tree" "$dir/$2" 2>&1)"
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
# A tree holding this proposal's own ADRs, for the rules that read them.
newtree adrs
cp -R "$here/../adr" "$tree/adr"; commit
adrtree="$tree"

echo "file-size/size.sh (limits + exceptions read from adr/file-size)"
run file-size size.sh "$(changeset "[$(file x-mock/internal/new.go A '' "$(nlines 150)")]")"
expect "a new 150-line file passes" pass
run file-size size.sh "$(changeset "[$(file x-mock/internal/new.go A '' "$(nlines 151)")]")"
expect "a new 151-line file is refused" refuse "over the limit of 150"
run file-size size.sh "$(changeset "[$(file x-mock/e2e/new_test.go A '' "$(nlines 400)")]")"
expect "a 400-line test passes" pass
run file-size size.sh "$(changeset "[$(file claude-mock/internal/runner/session.go M "$(nlines 701)" "$(nlines 702)")]")"
expect "an exception that grows is refused" refuse "may not grow (701 → 702 lines)"
run file-size size.sh "$(changeset "[$(file claude-mock/internal/runner/session.go M "$(nlines 701)" "$(nlines 650)")]")"
expect "an exception that shrinks passes" pass
run_gate gate/file-size ../../file-guard/file-size/size.sh "$(jq -n -c --arg n "$(nlines 151)" '{event: {kind: "PreFileCreate", path: "a/b.go", newContent: $n, resultKnown: true}}')"
expect "gate: a Write of 151 lines is refused before it lands" refuse "over the limit of 150"
run_gate gate/file-size ../../file-guard/file-size/size.sh "$(jq -n -c '{event: {kind: "PreFileUpdate", path: "a/b.go", oldContent: "x", newContent: "", resultKnown: false}}')"
expect "gate: an unpredictable edit is left to the commit check" pass
tree="$work/cov"
run file-size size.sh "$(changeset "[$(file x-mock/internal/new.go A '' "$(nlines 151)")]")"
expect "no ADR linking the rule: refuses rather than guessing limits" refuse "no ADR linking file-guard/file-size declares limits"
tree="$adrtree"

# ---------------------------------------------------------------------------
echo "layering/imports.sh"
newtree layer
mkdir -p "$tree/core/c" "$tree/a-mock/x" "$tree/b-mock/y"
printf 'module example.com/m\n\ngo 1.22\n' >"$tree/go.mod"
printf 'package c\n' >"$tree/core/c/c.go"
printf 'package y\n' >"$tree/b-mock/y/y.go"
printf 'package x\n\nimport _ "example.com/m/core/c"\n' >"$tree/a-mock/x/x.go"
commit
run layering imports.sh "$(changeset "[$(file a-mock/x/x.go M '' '')]")"
expect "a mock importing core passes" pass
printf 'package x\n\nimport _ "example.com/m/b-mock/y"\n' >"$tree/a-mock/x/x.go"; commit
run layering imports.sh "$(changeset "[$(file a-mock/x/x.go M '' '')]")"
expect "a mock importing another mock is refused" refuse "(another mock, b-mock)"
printf 'package x\n' >"$tree/a-mock/x/x.go"
printf 'package c\n\nimport _ "example.com/m/a-mock/x"\n' >"$tree/core/c/c.go"; commit
run layering imports.sh "$(changeset "[$(file core/c/c.go M '' '')]")"
expect "core importing a mock is refused" refuse "(core) imports example.com/m/a-mock/x"

# ---------------------------------------------------------------------------
echo "module-boundaries/imports-through-api.sh (adr/hooks-module: home core/hooks/**, api core/hooks)"
mkdir -p "$tree/adr" "$tree/core/hooks/match" && cp -R "$here/../adr/hooks-module" "$tree/adr/"
printf 'package c\n' >"$tree/core/c/c.go"
printf 'package hooks\n\nimport _ "example.com/m/core/hooks/match"\n' >"$tree/core/hooks/hooks.go"
printf 'package match\n' >"$tree/core/hooks/match/match.go"
printf 'package x\n\nimport _ "example.com/m/core/hooks"\n' >"$tree/a-mock/x/x.go"; commit
run module-boundaries imports-through-api.sh "$(changeset "[$(file a-mock/x/x.go M '' '')]")"
expect "using a module through its api, and the module using its own insides, pass" pass
printf 'package x\n\nimport _ "example.com/m/core/hooks/match"\n' >"$tree/a-mock/x/x.go"; commit
run module-boundaries imports-through-api.sh "$(changeset "[$(file a-mock/x/x.go M '' '')]")"
expect "reaching into a module past its api is refused" refuse "a-mock/x imports core/hooks/match, inside adr/hooks-module's module but not its api"

# ---------------------------------------------------------------------------
echo "subprocess-env/no-own-env.sh (exceptions read from adr/subprocess-env)"
tree="$adrtree"
run subprocess-env no-own-env.sh "$(changeset "[$(file claude-mock/internal/runner/newproc.go A '' $'package runner\n\tcmd.Env = append(os.Environ(), "X=1")')]")"
expect "a new site assigning cmd.Env is refused" refuse "claude-mock/internal/runner/newproc.go assigns cmd.Env"
run subprocess-env no-own-env.sh "$(changeset "[$(file core/procenv/env.go A '' $'package procenv\n\tcmd.Env = env')]")"
expect "core/procenv may assign it" pass
run subprocess-env no-own-env.sh "$(changeset "[$(file claude-mock/internal/runner/runner.go M $'\tcmd.Env = buildEnv(cfg, tr)' $'\tcmd.Env = buildEnv(cfg, tr)\n\tx := 1')]")"
expect "a legacy site that keeps its one assignment passes" pass
run subprocess-env no-own-env.sh "$(changeset "[$(file claude-mock/internal/runner/runner.go M $'\tcmd.Env = buildEnv(cfg, tr)' $'\tcmd.Env = buildEnv(cfg, tr)\n\tc2.Env = other()')]")"
expect "a legacy site that adds one is refused" refuse "adds a cmd.Env assignment (1 → 2)"
run subprocess-env no-own-env.sh "$(changeset "[$(file claude-mock/internal/runner/newproc.go A '' $'\tif cmd.Env == nil {}')]")"
expect "a comparison is not an assignment" pass

# ---------------------------------------------------------------------------
echo "capability-once/catalog-matches-markers.sh (spec/capabilities.yaml ⇄ markers)"
newtree caps
mkdir -p "$tree/spec" "$tree/core/stop" "$tree/claude-mock/adapter" "$tree/cursor-mock/adapter"
cat >"$tree/spec/capabilities.yaml" <<'YAML'
capabilities:
  - id: stop.block
    statement: s
    providers: {claude: supported, cursor: {n/a: "cursor's stop hook cannot block"}}
YAML
printf 'package stop\n// sr:capability stop.block\n' >"$tree/core/stop/stop.go"
printf 'package adapter\n// sr:provides stop.block claude\n' >"$tree/claude-mock/adapter/stop.go"
printf 'package adapter\n' >"$tree/cursor-mock/adapter/a.go"
commit
p="$(changeset "[$(file core/stop/stop.go M '' '')]")"
run capability-once catalog-matches-markers.sh "$p"; expect "catalog, core implementation and adapters agree" pass
sed -i '' '/sr:capability/d' "$tree/core/stop/stop.go"; commit
run capability-once catalog-matches-markers.sh "$p"; expect "a catalogued capability with no marked implementation is refused" refuse "capability 'stop.block' has no implementation"
git -C "$tree" reset -q --hard HEAD~1
sed -i '' '/sr:provides/d' "$tree/claude-mock/adapter/stop.go"; commit
run capability-once catalog-matches-markers.sh "$p"; expect "a supported cell with no adapter is refused" refuse "supported by 'claude' but no claude-mock/ code carries // sr:provides stop.block claude"
git -C "$tree" reset -q --hard HEAD~1
printf '  - {id: compact, statement: s, providers: {claude: supported}}\n' >>"$tree/spec/capabilities.yaml"; commit
run capability-once catalog-matches-markers.sh "$p"; expect "a missing cell is refused" refuse "capability 'compact' has no cell for harness 'cursor'"
git -C "$tree" reset -q --hard HEAD~1
printf '// sr:provides stop.block cursor\n' >>"$tree/cursor-mock/adapter/a.go"; commit
run capability-once catalog-matches-markers.sh "$p"; expect "providing an n/a cell is refused" refuse "is n/a for 'cursor', yet cursor-mock/ code provides it"
git -C "$tree" reset -q --hard HEAD~1
printf '// sr:capability undeclared.thing\n' >>"$tree/core/stop/stop.go"; commit
run capability-once catalog-matches-markers.sh "$p"; expect "a marker for an uncatalogued capability is refused" refuse "sr:capability 'undeclared.thing' is not in spec/capabilities.yaml"
git -C "$tree" reset -q --hard HEAD~1
printf '// sr:capability stop.block\n' >>"$tree/cursor-mock/adapter/a.go"; commit
run capability-once catalog-matches-markers.sh "$p"; expect "a second implementation outside core is refused" refuse "implemented in more than one place"

# ---------------------------------------------------------------------------
echo "adr-linked/links-resolve.sh"
newtree linked
cp -R "$here/../adr" "$tree/adr"; cp -R "$rules" "$tree/.sloprail"; commit
run adr-linked links-resolve.sh "$(changeset "[$(file adr/file-size/ADR.md M '' '')]")"
expect "this proposal's own ADRs are well-formed and every link resolves" pass
rm -rf "$tree/.sloprail/file-guard/layering"; commit
run adr-linked links-resolve.sh "$(changeset "[$(file .sloprail/file-guard/layering/file-guard.yaml D '' '')]")"
expect "deleting a rule an ADR links is refused" refuse "adr/layering links 'file-guard/layering', but .sloprail/file-guard/layering/file-guard.yaml does not exist"
git -C "$tree" reset -q --hard HEAD~1
sed -i '' 's/^sloprails: .*/sloprails: []/' "$tree/adr/layering/ADR.md"; commit
run adr-linked links-resolve.sh "$(changeset "[$(file adr/layering/ADR.md M '' '')]")"
expect "an ADR linking no sloprail is refused" refuse "adr/layering links no sloprail"
git -C "$tree" reset -q --hard HEAD~1
sed -i '' 's/^## Decision/## Outcome/' "$tree/adr/layering/ADR.md"
sed -i '' '2i\
status: accepted
' "$tree/adr/layering/ADR.md"; commit
run adr-linked links-resolve.sh "$(changeset "[$(file adr/layering/ADR.md M '' '')]")"
expect "a missing section is refused" refuse "adr/layering has no '## Decision' section"
expect "a status is refused: an ADR in the tree is in force" refuse "adr/layering has a status"
git -C "$tree" reset -q --hard HEAD~1
sed -i '' '/^concern:/d' "$tree/adr/layering/ADR.md"; commit
run adr-linked links-resolve.sh "$(changeset "[$(file adr/layering/ADR.md M '' '')]")"
expect "an ADR without a one-line concern is refused" refuse "adr/layering needs 'concern:'"
git -C "$tree" reset -q --hard HEAD~1
git -C "$tree" mv adr/layering adr/0002-layering; commit
run adr-linked links-resolve.sh "$(changeset "[$(file adr/0002-layering/ADR.md A '' '')]")"
expect "a numbered ADR folder is refused" refuse "adr/0002-layering: the folder name must be kebab-case starting with a letter"
git -C "$tree" reset -q --hard HEAD~1
printf -- '---\nsloprails: [file-guard/layering\n---\n' >"$tree/adr/layering/ADR.md"; commit
run adr-linked links-resolve.sh "$(changeset "[$(file adr/layering/ADR.md M '' '')]")"
expect "unparseable frontmatter is refused, not skipped" refuse "frontmatter that is not valid YAML"
git -C "$tree" reset -q --hard HEAD~1
mkdir -p "$tree/adr/history-in-adr" && cp "$here/judge-cases/adr-well-formed/history-in-adr/ADR.md" "$tree/adr/history-in-adr/"; commit
run adr-linked links-resolve.sh "$(changeset "[$(file adr/history-in-adr/ADR.md A '' '')]")"
expect "judge case history-in-adr is format-valid, so only adr-well-formed's judge can catch it" pass
git -C "$tree" reset -q --hard HEAD~1

# ---------------------------------------------------------------------------
echo "adr-matches-sloprails/subjects.sh"
run adr-matches-sloprails subjects.sh "$(changeset "[$(file .sloprail/file-guard/subprocess-env/no-own-env.sh M a b)]")"
got="$(printf '%s' "$out" | jq -r '[.subjects[] | "\(.id):\([.context.rules[].path | select(endswith("no-own-env.sh"))] | length)"] | join(",")' 2>/dev/null)"
[ "$got" = "subprocess-env:1" ] && ok "changing a rule re-judges the ADR that links it, with the rule's files" || bad "adr-matches subjects (rule change)" "got '$got' ($out)"
run adr-matches-sloprails subjects.sh "$(changeset "[$(file adr/file-size/ADR.md M a b)]")"
got="$(printf '%s' "$out" | jq -r '[.subjects[] | "\(.id):\([.context.rules[].rule] | unique | join("+"))"] | join(",")' 2>/dev/null)"
[ "$got" = "file-size:file-guard/file-size+gate/file-size" ] && ok "changing an ADR re-judges it against every rule it links (many-to-many)" || bad "adr-matches subjects (adr change)" "got '$got' ($out)"

# ---------------------------------------------------------------------------
echo "adr-grounded/needs-words.sh · subjects.sh"
adr=adr/file-size/ADR.md
run adr-grounded needs-words.sh "$(changeset "[$(file "$adr" M x y $'--- a\n+++ b\n@@\n-  - claude-mock/run.go   # 238\n')]")"
[ "$rc" -eq 1 ] && ok "shrinking an exception list needs no words (waived)" || bad "needs-words shrink" "rc=$rc $out"
run adr-grounded needs-words.sh "$(changeset "[$(file "$adr" M x y $'--- a\n+++ b\n@@\n+  - core/big.go\n')]")"
[ "$rc" -eq 0 ] && ok "adding an exception needs words" || bad "needs-words add" "rc=$rc $out"
run adr-grounded needs-words.sh "$(changeset "[$(file adr/new-thing/ADR.md A '' y $'+new')]")"
[ "$rc" -eq 0 ] && ok "a new ADR needs words" || bad "needs-words new" "rc=$rc $out"
run adr-grounded subjects.sh "$(changeset "[$(file "$adr" M old new d1), $(file adr/subprocess-env/ADR.md A '' n3 d3)]")"
got="$(printf '%s' "$out" | jq -r '[.subjects[] | "\(.id):\(.context.adr_after)"] | join(",")' 2>/dev/null)"
[ "$got" = "file-size:new,subprocess-env:n3" ] && ok "one subject per ADR touched" || bad "adr-grounded subjects" "got '$got' ($out)"

# ---------------------------------------------------------------------------
echo "adr-well-formed/subjects.sh · concern-placement/adr-index.sh"
run adr-well-formed subjects.sh "$(changeset "[$(file adr/layering/ADR.md M a b d), $(file adr/layering/notes.md A '' x)]")"
got="$(printf '%s' "$out" | jq -r '[.subjects[].id] | join(",")' 2>/dev/null)"
[ "$got" = "layering" ] && ok "only a changed ADR.md is judged for form" || bad "adr-well-formed subjects" "got '$got' ($out)"
run concern-placement adr-index.sh "$(changeset '[]')"
got="$(printf '%s' "$out" | jq -r '[.additionalContext.adrs[] | select(.home | length > 0) | "\(.id)=\(.home | join(","))"] | join(" ")' 2>/dev/null)"
[ "$got" = "capability-once=core/** hooks-module=core/hooks/**" ] && ok "the placement judge gets every ADR's home" || bad "adr-index homes" "got '$got' ($out)"
got="$(printf '%s' "$out" | jq -r '[.additionalContext.adrs[] | keys | join(",")] | unique | join(" ")' 2>/dev/null)"
[ "$got" = "concern,home,id" ] && ok "…as an index (id, concern, home), never the full ADR text" || bad "adr-index shape" "got '$got'"

echo
echo "$pass passed, $fail failed"
[ "$fail" -eq 0 ]
