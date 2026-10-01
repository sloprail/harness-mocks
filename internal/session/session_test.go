package session

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/sloprail/harness-mocks/internal/transcript"
)

var nonAlnum = regexp.MustCompile(`[^a-zA-Z0-9]`)

func layout() transcript.Layout {
	return transcript.Layout{ProjectsDir: "projects", Ext: ".jsonl", Encode: func(d string) string { return nonAlnum.ReplaceAllString(d, "-") }}
}

func write(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestFind_PrefersTheWorkingDirectoryThenAnyProject(t *testing.T) {
	cfg, here, there := t.TempDir(), t.TempDir(), t.TempDir()
	l := layout()
	write(t, l.FilePath(cfg, there, "s"))
	if got := Find(l, cfg, here, "s"); got != l.FilePath(cfg, there, "s") {
		t.Fatalf("Find from another directory = %q", got)
	}
	write(t, l.FilePath(cfg, here, "s"))
	if got := Find(l, cfg, here, "s"); got != l.FilePath(cfg, here, "s") {
		t.Fatalf("Find in the working directory = %q", got)
	}
}

func TestFind_NewestWinsAndBadIDsFindNothing(t *testing.T) {
	cfg, a, b, c := t.TempDir(), t.TempDir(), t.TempDir(), t.TempDir()
	l := layout()
	write(t, l.FilePath(cfg, a, "s"))
	write(t, l.FilePath(cfg, b, "s"))
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(l.FilePath(cfg, a, "s"), old, old); err != nil {
		t.Fatal(err)
	}
	if got := Find(l, cfg, c, "s"); got != l.FilePath(cfg, b, "s") {
		t.Fatalf("Find = %q, want the most recently written", got)
	}
	for _, id := range []string{"", ".", "..", "../s", `a\b`, "missing"} {
		if got := Find(l, cfg, c, id); got != "" {
			t.Fatalf("Find(%q) = %q", id, got)
		}
	}
}

func TestResume_ReportsThePathUnderTheResumingDirectory(t *testing.T) {
	cfg, begun, resumed := t.TempDir(), t.TempDir(), t.TempDir()
	l := layout()
	write(t, l.FilePath(cfg, begun, "s"))
	r, err := Resume(l, cfg, resumed, "s")
	if err != nil {
		t.Fatal(err)
	}
	if r.Path != l.FilePath(cfg, begun, "s") || r.Reported != l.FilePath(cfg, resumed, "s") {
		t.Fatalf("Resume = %+v", r)
	}
}

func TestResume_UnknownSessionFailsAndFiresOnlyTheEnd(t *testing.T) {
	_, err := Resume(layout(), t.TempDir(), t.TempDir(), "nope")
	var nc *NoConversationError
	if !errors.As(err, &nc) || nc.SessionID != "nope" {
		t.Fatalf("err = %v", err)
	}
	if len(nc.Fire) != 1 || nc.Fire[0] != End {
		t.Fatalf("Fire = %v, want only the end of the session", nc.Fire)
	}
}

func rec(uuid, parent string, extra map[string]any) Record {
	r := Record{"uuid": uuid, "parentUuid": parent, "sessionId": "old"}
	for k, v := range extra {
		r[k] = v
	}
	return r
}

func schema() ForkSchema {
	return ForkSchema{
		UUID: "uuid", Parent: "parentUuid", SessionKey: "sessionId",
		IsBoundary: func(r Record) bool { return r["subtype"] == "boundary" },
		IsSummary:  func(r Record) bool { return r["summary"] == true },
		Preserved: func(b Record) []string {
			var out []string
			for _, v := range b["keep"].([]string) {
				out = append(out, v)
			}
			return out
		},
	}
}

func uuids(rs []Record) []string {
	var out []string
	for _, r := range rs {
		out = append(out, r["uuid"].(string))
	}
	return out
}

func TestFork_UncompactedSessionForksWholeUnderTheNewID(t *testing.T) {
	recs := []Record{rec("a", "", nil), rec("b", "a", nil), rec("c", "b", nil)}
	got := Fork(recs, "new", schema())
	if len(got) != 3 || got[1]["parentUuid"] != "a" {
		t.Fatalf("fork = %v", got)
	}
	for _, r := range got {
		if r["sessionId"] != "new" {
			t.Fatalf("record %v keeps the old session", r)
		}
	}
}

func TestFork_CompactedSessionForksFromItsLastBoundary(t *testing.T) {
	recs := []Record{
		rec("a", "", nil), rec("b", "a", nil), rec("c", "b", nil),
		rec("bound", "", map[string]any{"subtype": "boundary", "keep": []string{"b", "c"}}),
		rec("sum", "bound", map[string]any{"summary": true}),
		rec("d", "sum", nil),
	}
	got := Fork(recs, "new", schema())
	want := []string{"bound", "sum", "b", "c", "d"}
	if g := uuids(got); len(g) != len(want) || g[0] != want[0] || g[1] != want[1] || g[2] != want[2] || g[3] != want[3] || g[4] != want[4] {
		t.Fatalf("fork order = %v, want %v", g, want)
	}
	if got[2]["parentUuid"] != "sum" || got[3]["parentUuid"] != "b" || got[4]["parentUuid"] != "c" {
		t.Fatalf("chain after the summary = %v", got)
	}
}

func TestParseRecords_DropsRecordsWithoutUUID(t *testing.T) {
	got := ParseRecords([]byte("{\"uuid\":\"a\"}\n{\"type\":\"mode\"}\nnot json\n{\"uuid\":\"b\"}\n"), "uuid")
	if g := uuids(got); len(g) != 2 || g[0] != "a" || g[1] != "b" {
		t.Fatalf("records = %v", g)
	}
}
