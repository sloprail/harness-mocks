package turnloop

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/sloprail/harness-mocks/internal/scenario"
)

// host records the turn, and answers the hooks from its fields.
type host struct {
	log        []string
	blocked    bool
	stops      []string // reasons the end-of-turn hooks block with, one per call
	nextStop   int
	cap        int
	sessionLog string
}

func (h *host) SubmitPrompt(context.Context) (string, bool) {
	h.log = append(h.log, "prompt")
	return "ctx", h.blocked
}
func (h *host) Say(text string) { h.log = append(h.log, "say:"+text) }
func (h *host) Tool(_ context.Context, tu scenario.ToolUse) {
	h.log = append(h.log, "tool:"+tu.Name)
	f, _ := os.OpenFile(h.sessionLog, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	fmt.Fprintln(f, "output "+tu.ID)
	f.Close()
}
func (h *host) EndOfTurn(_ context.Context, last string, continuing bool) (string, bool) {
	h.log = append(h.log, fmt.Sprintf("stop:%s:%v", last, continuing))
	if h.nextStop >= len(h.stops) {
		return "", false
	}
	h.nextStop++
	return h.stops[h.nextStop-1], true
}
func (h *host) Continue(reason string) { h.log = append(h.log, "continue:"+reason) }
func (h *host) CapOverridden(blocks int) {
	h.log = append(h.log, fmt.Sprintf("overridden:%d", blocks))
}
func (h *host) SessionFile() string { return h.sessionLog }

// run drives the loop with a script that asks for one tool call per output
// recorded in the session file, then gives its result.
func run(t *testing.T, h *host, calls int, body string) (string, error) {
	t.Helper()
	dir := t.TempDir()
	h.sessionLog = filepath.Join(dir, "session")
	if body == "" {
		body = fmt.Sprintf(`n=$(grep -c '^output' "$A10N_MOCK_SESSION_FILE" 2>/dev/null); n=${n:-0}
if [ "$n" -lt %d ]; then
  printf '{"type":"assistant","message":{"content":[{"type":"tool_use","id":"c%%s","name":"Bash","input":{"n":%%s}}]}}\n' "$n" "$n"
  exit 0
fi
printf '%%s\n' '{"type":"assistant","message":{"content":[{"type":"text","text":"done"}]}}' '{"type":"result","result":"done"}'`, calls)
	}
	script := filepath.Join(dir, "s.sh")
	if err := os.WriteFile(script, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return Run(context.Background(), h, Params{Script: script, Dir: dir, Environ: []string{"PATH=/usr/bin:/bin"}, Prompt: "go", BlockCap: h.cap})
}

func TestRunStepsThroughToolCallsToTheResult(t *testing.T) {
	h := &host{}
	last, err := run(t, h, 2, "")
	want := []string{"prompt", "tool:Bash", "tool:Bash", "say:done", "stop:done:false"}
	if err != nil || last != "done" || !reflect.DeepEqual(h.log, want) {
		t.Fatalf("last=%q err=%v log=%v, want %v", last, err, h.log, want)
	}
}

func TestRunBlockedPromptEndsTheTurnWithoutTheAgent(t *testing.T) {
	h := &host{blocked: true}
	last, err := run(t, h, 1, "")
	if err != nil || last != "" || !reflect.DeepEqual(h.log, []string{"prompt"}) {
		t.Fatalf("last=%q err=%v log=%v: the script must not run", last, err, h.log)
	}
}

func TestRunEndOfTurnBlockContinuesTheTurnWithItsReason(t *testing.T) {
	h := &host{stops: []string{"again", "once more"}}
	_, err := run(t, h, 0, "")
	want := []string{"prompt", "say:done", "stop:done:false", "continue:again", "say:done", "stop:done:true",
		"continue:once more", "say:done", "stop:done:true"}
	if err != nil || !reflect.DeepEqual(h.log, want) {
		t.Fatalf("err=%v log=%v, want %v", err, h.log, want)
	}
}

// sr:proves loop-guard
func TestRunSameToolCallFiveTimesInARowAbortsTheRun(t *testing.T) {
	h := &host{}
	_, err := run(t, h, 0, `printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"tool_use","id":"x","name":"Bash","input":{"a":1}}]}}'`)
	if err == nil || !strings.Contains(err.Error(), "5 turns in a row") {
		t.Fatalf("err = %v, want the loop guard", err)
	}
	if n := strings.Count(strings.Join(h.log, " "), "tool:Bash"); n != LoopLimit-1 {
		t.Fatalf("%d calls ran before the abort, want %d", n, LoopLimit-1)
	}
}

func TestRunDifferentCallsAreNotALoop(t *testing.T) {
	h := &host{}
	if _, err := run(t, h, 8, ""); err != nil {
		t.Fatalf("eight different calls: %v", err)
	}
}
