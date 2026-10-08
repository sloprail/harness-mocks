package turnloop

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/sloprail/harness-mocks/internal/scenario"
)

// thinkingHost says its thoughts in the log, in order with what else it is told.
type thinkingHost struct{ host }

func (h *thinkingHost) Think(_ context.Context, th scenario.Thought, took time.Duration) {
	h.log = append(h.log, "think:"+th.Text)
	if took < 0 {
		h.log = append(h.log, "negative")
	}
}

// A thought of a turn is told before the turn's message, and a host that does not
// think is not told.
func TestRunTellsAThinkerItsThoughtsBeforeTheMessage(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "s.sh")
	body := `printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"thinking","thinking":"hm"},{"type":"text","text":"done"}]}}' '{"type":"result","result":"done"}'`
	if err := os.WriteFile(script, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	h := &thinkingHost{host{sessionLog: filepath.Join(dir, "session")}}
	if _, err := Run(context.Background(), h, Params{Script: script, Dir: dir, Environ: []string{"PATH=/usr/bin:/bin"}, Prompt: "go"}); err != nil {
		t.Fatal(err)
	}
	want := []string{"prompt", "think:hm", "say:done", "stop:done:false"}
	if !reflect.DeepEqual(h.log, want) {
		t.Fatalf("log = %v, want %v", h.log, want)
	}
	plain := &host{sessionLog: filepath.Join(dir, "session2")}
	if _, err := Run(context.Background(), plain, Params{Script: script, Dir: dir, Environ: []string{"PATH=/usr/bin:/bin"}, Prompt: "go"}); err != nil {
		t.Fatal(err)
	}
	if want := []string{"prompt", "say:done", "stop:done:false"}; !reflect.DeepEqual(plain.log, want) {
		t.Fatalf("log = %v, want %v", plain.log, want)
	}
}
