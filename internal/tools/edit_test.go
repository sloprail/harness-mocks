package tools

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestEdit(t *testing.T) {
	for _, tc := range []struct {
		name, content, old, new string
		all                     bool
		want                    string
		matches                 int
		err                     error
	}{
		{"one match", "alpha\nbeta\nalpha\n", "beta", "BETA", false, "alpha\nBETA\nalpha\n", 1, nil},
		{"no match", "alpha\n", "zeta", "z", false, "alpha\n", 0, ErrNoMatch},
		{"empty old string", "alpha\n", "", "z", false, "alpha\n", 0, ErrNoMatch},
		{"two matches", "alpha\nbeta\nalpha\n", "alpha", "A", false, "alpha\nbeta\nalpha\n", 2, ErrAmbiguous},
		{"replace all", "alpha\nbeta\nalpha\n", "alpha", "A", true, "A\nbeta\nA\n", 2, nil},
		{"exact, not a pattern", "a.c abc\n", "a.c", "X", false, "X abc\n", 1, nil},
		{"whitespace counts", "  x\n", "x", "y", false, "  y\n", 1, nil},
	} {
		got, n, err := Edit(tc.content, tc.old, tc.new, tc.all)
		if got != tc.want || n != tc.matches || !errors.Is(err, tc.err) {
			t.Errorf("%s: Edit = (%q, %d, %v), want (%q, %d, %v)", tc.name, got, n, err, tc.want, tc.matches, tc.err)
		}
	}
}

func TestPatchIsASingleHunkWithContext(t *testing.T) {
	for _, tc := range []struct {
		name, old, new string
		want           []Hunk
	}{
		{"equal", "a\nb\n", "a\nb\n", nil},
		{"an edit", "alpha\nbeta\nalpha\n", "alpha\nBETA\nalpha\n",
			[]Hunk{{1, 3, 1, 3, []string{" alpha", "-beta", "+BETA", " alpha"}}}},
		{"a rewrite", "alpha\nBETA\nalpha\n", "replaced\n",
			[]Hunk{{1, 3, 1, 1, []string{"-alpha", "-BETA", "-alpha", "+replaced"}}}},
		{"a new file", "", "a\nb\n", []Hunk{{1, 0, 1, 2, []string{"+a", "+b"}}}},
		{"an emptied file", "a\n", "", []Hunk{{1, 1, 1, 0, []string{"-a"}}}},
		{"context stops at three lines", "1\n2\n3\n4\n5\n6\n7\n8\n9\n", "1\n2\n3\n4\nX\n6\n7\n8\n9\n",
			[]Hunk{{2, 7, 2, 7, []string{" 2", " 3", " 4", "-5", "+X", " 6", " 7", " 8"}}}},
		{"an added line", "a\nb\n", "a\nnew\nb\n", []Hunk{{1, 2, 1, 3, []string{" a", "+new", " b"}}}},
	} {
		if got := Patch(tc.old, tc.new); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: Patch = %+v, want %+v", tc.name, got, tc.want)
		}
	}
}

func TestRead(t *testing.T) {
	for _, tc := range []struct {
		name, content string
		offset, limit int
		want          View
	}{
		{"whole file, its final newline a last empty line", "alpha\nbeta\nalpha\n", 0, 0, View{"alpha\nbeta\nalpha\n", 1, 4, 4}},
		{"an offset and a limit", "alpha\nbeta\nalpha\n", 2, 1, View{"beta", 2, 1, 4}},
		{"an offset alone reads to the end", "a\nb\nc", 2, 0, View{"b\nc", 2, 2, 3}},
		{"a limit alone reads from the start", "a\nb\nc", 0, 2, View{"a\nb", 1, 2, 3}},
		{"a limit past the end", "a\nb", 2, 9, View{"b", 2, 1, 2}},
		{"an empty file is one empty line", "", 0, 0, View{"", 1, 1, 1}},
		{"an offset past the last line", "a\nb", 5, 0, View{"", 5, 0, 2}},
	} {
		if got := Read(tc.content, tc.offset, tc.limit); got != tc.want {
			t.Errorf("%s: Read = %+v, want %+v", tc.name, got, tc.want)
		}
	}
}

func TestGlob(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	for i, f := range []string{"old.txt", "a/deep/new.txt", "a/b.json", "a/c.yaml", "top.go", "a/deep/more.txt"} {
		p := filepath.Join(dir, f)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, nil, 0o644); err != nil {
			t.Fatal(err)
		}
		// later in the list, newer
		if err := os.Chtimes(p, now, now.Add(time.Duration(i)*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		pattern string
		limit   int
		want    []string
		total   int
		trunc   bool
	}{
		{"*.txt", 0, []string{"old.txt"}, 1, false},
		{"**/*.txt", 0, []string{"old.txt", "a/deep/new.txt", "a/deep/more.txt"}, 3, false},
		{"a/**/*.txt", 0, []string{"a/deep/new.txt", "a/deep/more.txt"}, 2, false},
		{"a/*.{json,yaml}", 0, []string{"a/b.json", "a/c.yaml"}, 2, false},
		{"a/?.json", 0, []string{"a/b.json"}, 1, false},
		{"**/*.txt", 2, []string{"old.txt", "a/deep/new.txt"}, 3, true},
		{"*.nothing", 0, nil, 0, false},
		{"a", 0, nil, 0, false}, // a directory is not a result
	} {
		got, err := Glob(dir, tc.pattern, tc.limit)
		if err != nil || !reflect.DeepEqual(got.Names, tc.want) || got.Total != tc.total || got.Truncated != tc.trunc {
			t.Errorf("Glob(%q, limit %d) = %+v, %v; want %v total %d truncated %v", tc.pattern, tc.limit, got, err, tc.want, tc.total, tc.trunc)
		}
	}
}

// A file that ends without a newline gets the unified diff's marker after its last line in the hunk
// (recorded: Edit's structuredPatch in claude runs/fgsub-tool-stats).
func TestPatchMarksAMissingFinalNewline(t *testing.T) {
	h := Patch("alpha\nbeta\ngamma", "alpha\ndelta\ngamma")
	want := []string{" alpha", "-beta", "+delta", " gamma", "\\ No newline at end of file"}
	if len(h) != 1 || !reflect.DeepEqual(h[0].Lines, want) {
		t.Fatalf("%+v", h)
	}
	if h := Patch("a\nb\n", "a\nc\n"); len(h) != 1 || len(h[0].Lines) != 3 {
		t.Fatalf("a file with its newline has no marker: %+v", h)
	}
}
