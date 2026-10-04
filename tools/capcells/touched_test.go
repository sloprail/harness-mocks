package capcells

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The two versions a snapshot used to share are separate: the harness binary a run was recorded
// with (run.yaml; MANIFEST `pin` is only which binary captures run next) and each doc page's own
// sha256. These run _lib/touched.sh's load_doc_changes and load_doc_shas on a MANIFEST before
// and after, and check that only a page whose sha256 moved is in question.

const (
	urlA = "https://d.example/a"
	urlB = "https://d.example/b"
	shaA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	shaB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	shaC = "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
)

type page struct{ url, sha, fetched string }

// manifest renders a MANIFEST at pin, freezing each page.
func manifest(pin string, pages ...page) string {
	var b strings.Builder
	b.WriteString("pin: " + pin + "\ndocs:\n")
	for _, p := range pages {
		b.WriteString("  " + p.url + ":\n    sha256: " + p.sha + "\n    fetched: \"" + p.fetched + "\"\n")
	}
	return b.String()
}

// legacy is the schema before the split: one global version, and one per page.
func legacy(version string, pages ...page) string {
	b := "version: \"" + version + "\"\ndocs:\n"
	for _, p := range pages {
		b += "  " + p.url + ":\n    version: " + version + "\n    sha256: " + p.sha + "\n"
	}
	return b
}

const man = "claude-mock/snapshots/MANIFEST.yaml"

func needTools(t *testing.T) {
	t.Helper()
	for _, bin := range []string{"jq", "yq", "git", "bash"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not installed", bin)
		}
	}
}

// touched runs the loader fn ("load_doc_changes" or "load_doc_shas") and prints the variable.
func touched(t *testing.T, tree, payload, fn, variable string) string {
	t.Helper()
	needTools(t)
	lib, err := filepath.Abs("../../.sloprail/_lib")
	if err != nil {
		t.Fatal(err)
	}
	script := `set -uo pipefail; payload="$(cat)"; . ` + lib + `/changeset.sh; . ` + lib + `/spec.sh; . ` + lib + `/snapshots.sh; . ` + lib +
		`/touched.sh; ` + fn + `; printf '%s' "$` + variable + `"`
	cmd := exec.Command("bash", "-c", script)
	cmd.Env = append(os.Environ(), "SR_TREE="+tree)
	cmd.Stdin = strings.NewReader(payload)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s: %v\n%s", fn, err, out)
	}
	return string(out)
}

func payloadOf(t *testing.T, status, old, new string) string {
	t.Helper()
	b, err := json.Marshal(map[string]any{"changeset": map[string]any{"files": []map[string]any{
		{"path": man, "status": status, "oldContent": old, "newContent": new}}}})
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func changes(t *testing.T, status, old, new string) string {
	t.Helper()
	return strings.TrimSpace(touched(t, t.TempDir(), payloadOf(t, status, old, new), "load_doc_changes", "DOC_CHANGES_TSV"))
}

var (
	pa1 = page{urlA, shaA, "2026-10-01"}
	pb1 = page{urlB, shaB, "2026-10-01"}
)

func TestABinaryBumpInvalidatesNothing(t *testing.T) {
	if got := changes(t, "M", manifest("2.1.285", pa1, pb1), manifest("2.1.288", pa1, pb1)); got != "" {
		t.Fatalf("a bumped pin must put no page in question, got %q", got)
	}
}

func TestRefreezingAPageWithTheSameShaInvalidatesNothing(t *testing.T) {
	again := page{urlA, shaA, "2026-10-09"}
	if got := changes(t, "M", manifest("2.1.285", pa1, pb1), manifest("2.1.285", again, pb1)); got != "" {
		t.Fatalf("a new fetch date with the same sha256 must put no page in question, got %q", got)
	}
	if got := changes(t, "M", manifest("2.1.285", pa1, pb1), manifest("2.1.288", again, pb1)); got != "" {
		t.Fatalf("a bump together with an unchanged re-freeze must put no page in question, got %q", got)
	}
}

func TestOnlyAPageWhoseShaChangedIsInQuestion(t *testing.T) {
	changed := page{urlA, shaC, "2026-10-09"}
	want := "claude\t" + urlA
	if got := changes(t, "M", manifest("2.1.285", pa1, pb1), manifest("2.1.285", changed, pb1)); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	// ... and the same when the binary moved in the same change
	if got := changes(t, "M", manifest("2.1.285", pa1, pb1), manifest("2.1.288", changed, pb1)); got != want {
		t.Fatalf("with a bump, got %q, want %q", got, want)
	}
}

func TestAnAddedOrRemovedPageIsInQuestion(t *testing.T) {
	if got := changes(t, "M", manifest("1", pa1), manifest("1", pa1, pb1)); got != "claude\t"+urlB {
		t.Fatalf("an added page: got %q", got)
	}
	if got := changes(t, "M", manifest("1", pa1, pb1), manifest("1", pa1)); got != "claude\t"+urlB {
		t.Fatalf("a removed page: got %q", got)
	}
}

func TestMigratingToTheSplitSchemaInvalidatesNothing(t *testing.T) {
	if got := changes(t, "M", legacy("2.1.285", pa1, pb1), manifest("2.1.285", pa1, pb1)); got != "" {
		t.Fatalf("dropping the global and per-page version keeps every sha256: nothing in question, got %q", got)
	}
}

func TestAnAddedOrUnreadableManifestPutsTheWholeHarnessInQuestion(t *testing.T) {
	if got := changes(t, "A", "", manifest("1", pa1)); got != "claude\t*" {
		t.Fatalf("an added MANIFEST: got %q", got)
	}
	if got := changes(t, "M", "not: [valid", manifest("1", pa1)); got != "claude\t*" {
		t.Fatalf("an unreadable old MANIFEST: got %q", got)
	}
}

func TestDocShasCarryNoVersion(t *testing.T) {
	needTools(t)
	root := t.TempDir()
	full := filepath.Join(root, man)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(manifest("2.1.285", pa1, pb1)), 0o644); err != nil {
		t.Fatal(err)
	}
	got := touched(t, root, `{"changeset":{"files":[]}}`, "load_doc_shas", "DOC_SHAS")
	var shas map[string]map[string]any
	if err := json.Unmarshal([]byte(got), &shas); err != nil {
		t.Fatalf("%v: %s", err, got)
	}
	c := shas["claude"]
	if _, has := c["version"]; has || len(c) != 1 {
		t.Fatalf("a verdict's key must carry page hashes only, not a binary version: %s", got)
	}
	docs := c["docs"].(map[string]any)
	if docs[urlA] != shaA || docs[urlB] != shaB {
		t.Fatalf("each cited page's hash must be in the key: %s", got)
	}
}
