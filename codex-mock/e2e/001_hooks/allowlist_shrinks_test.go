package e2e

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

// notReplaying only shrinks: it may lose entries (a run now replays green) and
// never gain one. A run that does not replay is fixed (the mock, the adapter or
// the recording), not listed. CI sets ALLOWLIST_BASE to the base branch (origin/main)
// and checks out its history; without it the test is skipped, as there is nothing
// to compare with.
func TestAllowlistOnlyShrinks(t *testing.T) {
	base := os.Getenv("ALLOWLIST_BASE")
	if base == "" {
		t.Skip("ALLOWLIST_BASE is not set: nothing to compare the allowlist with")
	}
	const path = "codex-mock/e2e/001_hooks/replay_allowlist_test.go"
	out, err := exec.Command("git", "show", base+":"+path).Output()
	if err != nil {
		t.Skipf("%s has no %s: nothing to compare with", base, path)
	}
	f, err := parser.ParseFile(token.NewFileSet(), path, out, 0)
	require.NoError(t, err)
	before := allowlistKeys(t, f)
	for name := range notReplaying {
		if !before[name] {
			t.Errorf("notReplaying gained %q (not in %s): the list only shrinks; make the run replay instead", name, base)
		}
	}
}

// allowlistKeys are the keys of the notReplaying map literal in f.
func allowlistKeys(t *testing.T, f *ast.File) map[string]bool {
	keys := map[string]bool{}
	ast.Inspect(f, func(n ast.Node) bool {
		vs, ok := n.(*ast.ValueSpec)
		if !ok || len(vs.Names) != 1 || vs.Names[0].Name != "notReplaying" || len(vs.Values) != 1 {
			return true
		}
		lit, ok := vs.Values[0].(*ast.CompositeLit)
		require.True(t, ok, "notReplaying is not a map literal")
		for _, e := range lit.Elts {
			kv := e.(*ast.KeyValueExpr)
			k, err := strconv.Unquote(kv.Key.(*ast.BasicLit).Value)
			require.NoError(t, err)
			keys[k] = true
		}
		return false
	})
	require.NotEmpty(t, keys, "no notReplaying keys found at the base")
	return keys
}
