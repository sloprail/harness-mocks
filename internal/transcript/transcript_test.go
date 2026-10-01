package transcript

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var nonAlnum = regexp.MustCompile(`[^a-zA-Z0-9]`)

func layout() Layout {
	return Layout{ProjectsDir: "projects", Ext: ".jsonl", Encode: func(d string) string { return nonAlnum.ReplaceAllString(d, "-") }}
}

func TestFilePath_KeysByResolvedDirAndSessionID(t *testing.T) {
	cfg := t.TempDir()
	real := t.TempDir()
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	resolved, _ := filepath.EvalSymlinks(real)
	want := filepath.Join(cfg, "projects", nonAlnum.ReplaceAllString(resolved, "-"), "s1.jsonl")
	if got := layout().FilePath(cfg, link, "s1"); got != want {
		t.Fatalf("FilePath through a symlink = %s, want %s", got, want)
	}
	if got := layout().FilePath(cfg, real, "s2"); filepath.Base(got) != "s2.jsonl" {
		t.Fatalf("FilePath = %s, want the session id as its file name", got)
	}
}

func TestResolveDir_MissingDirectoryIsKept(t *testing.T) {
	if got := ResolveDir("/nonexistent/dir"); got != "/nonexistent/dir" {
		t.Fatalf("ResolveDir = %s", got)
	}
}

func TestLazy_FileDoesNotExistUntilFirstUse(t *testing.T) {
	path := filepath.Join(t.TempDir(), "p", "s.jsonl")
	l, err := NewLazy(path, "")
	if err != nil {
		t.Fatal(err)
	}
	if l.Exists() {
		t.Fatal("a fresh session's file exists before anything is written")
	}
	if l.Reported != path {
		t.Fatalf("Reported = %s, want the path itself by default", l.Reported)
	}
	f := l.File(func() [][]byte { return [][]byte{[]byte(`{"head":1}`)} })
	if f == nil || !l.Exists() {
		t.Fatal("File did not create the file")
	}
	l.Close()
	data, _ := os.ReadFile(path)
	if string(data) != "{\"head\":1}\n" {
		t.Fatalf("file = %q, want the head line written when this call created it", data)
	}
}

func TestLazy_ExistingFileGetsNoHead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.jsonl")
	if err := os.WriteFile(path, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	l, err := NewLazy(path, "elsewhere")
	if err != nil {
		t.Fatal(err)
	}
	l.File(func() [][]byte { return [][]byte{[]byte("HEAD")} })
	l.Close()
	data, _ := os.ReadFile(path)
	if string(data) != "{}\n" || l.Reported != "elsewhere" {
		t.Fatalf("file = %q, reported = %s", data, l.Reported)
	}
}

func TestEnvelopeStamp_FillsOnlyMissingFields(t *testing.T) {
	e := Envelope{"sessionId": "S", "version": "1"}
	out := e.Stamp([]byte(`{"type":"user","sessionId":"OWN"}`))
	var rec map[string]any
	if err := json.Unmarshal(out, &rec); err != nil {
		t.Fatal(err)
	}
	if rec["sessionId"] != "OWN" || rec["version"] != "1" || rec["type"] != "user" {
		t.Fatalf("stamped = %v", rec)
	}
	same := []byte(`{"sessionId":"OWN","version":"2"}`)
	if string(e.Stamp(same)) != string(same) {
		t.Fatal("a record with every field changed")
	}
	if string(e.Stamp([]byte("not json"))) != "not json" {
		t.Fatal("a line that is not an object was changed")
	}
}

func TestMarkPrompt_OnlyNonInteractive(t *testing.T) {
	marks := map[string]any{"source": "sdk"}
	typed := map[string]any{}
	MarkPrompt(typed, false, marks)
	if len(typed) != 0 {
		t.Fatalf("an interactive prompt was marked: %v", typed)
	}
	scripted := map[string]any{"source": "own"}
	MarkPrompt(scripted, true, map[string]any{"source": "sdk", "origin": "sdk"})
	if scripted["source"] != "own" || scripted["origin"] != "sdk" {
		t.Fatalf("marked = %v", scripted)
	}
}

func TestChain_MintsUUIDAndLinksParent(t *testing.T) {
	n := 0
	mint := func() string { n++; return "u" + string(rune('0'+n)) }
	out, uuid := Chain([]byte(`{"type":"user"}`), "p", "uuid", "parentUuid", mint)
	if uuid != "u1" || !strings.Contains(string(out), `"parentUuid":"p"`) {
		t.Fatalf("out=%s uuid=%s", out, uuid)
	}
	first, _ := Chain([]byte(`{"type":"user"}`), "", "uuid", "parentUuid", mint)
	if !strings.Contains(string(first), `"parentUuid":null`) {
		t.Fatalf("first record = %s, want an explicit null parent", first)
	}
	own, uuid := Chain([]byte(`{"uuid":"mine","parentUuid":null}`), "p", "uuid", "parentUuid", mint)
	if uuid != "mine" || string(own) != `{"uuid":"mine","parentUuid":null}` {
		t.Fatalf("own = %s", own)
	}
}

func TestMarshal_DoesNotEscapeHTML(t *testing.T) {
	b, err := Marshal(map[string]string{"a": "<x> && y"})
	if err != nil || string(b) != `{"a":"<x> && y"}` {
		t.Fatalf("Marshal = %s, %v", b, err)
	}
}
