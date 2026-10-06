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

// ctxHost is a host whose hooks add context as the turn goes: after each tool.
type ctxHost struct {
	host
	added []string
}

func (h *ctxHost) Tool(ctx context.Context, tu scenario.ToolUse) {
	h.host.Tool(ctx, tu)
	h.added = append(h.added, "after-"+tu.ID)
}

func TestRunGivesTheScriptTheContextAHostAddsAsTheTurnGoes(t *testing.T) {
	dir := t.TempDir()
	h := &ctxHost{host: host{sessionLog: filepath.Join(dir, "session")}}
	script := filepath.Join(dir, "s.sh")
	body := `n=$(grep -c '^output' "$A10N_MOCK_SESSION_FILE" 2>/dev/null); n=${n:-0}
printf '%s\n' "$A10N_MOCK_ADDITIONAL_CONTEXT" | tr '\n' ',' >>"$A10N_MOCK_SESSION_FILE.ctx"; echo >>"$A10N_MOCK_SESSION_FILE.ctx"
if [ "$n" -lt 2 ]; then
  printf '{"type":"assistant","message":{"content":[{"type":"tool_use","id":"c%s","name":"Bash","input":{"n":%s}}]}}\n' "$n" "$n"
  exit 0
fi
printf '%s\n' '{"type":"result","result":"done"}'`
	if err := os.WriteFile(script, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(context.Background(), h, Params{Script: script, Dir: dir, Environ: []string{"PATH=/usr/bin:/bin"}, Prompt: "go",
		Added: func() string { return strings.Join(h.added, "\n") }}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(h.sessionLog + ".ctx")
	if err != nil {
		t.Fatal(err)
	}
	want := "ctx,\nctx,after-c0,\nctx,after-c0,after-c1,\n"
	if string(got) != want {
		t.Fatalf("the script was given %q, want %q: the prompt hooks' context, then what the host added so far", got, want)
	}
}

// gateHost holds a turn back as a Gater: it logs the gate it was asked to hold, before the turn's call is taken.
type gateHost struct {
	host
	gates []scenario.Gate
}

func (h *gateHost) Gate(_ context.Context, g scenario.Gate) {
	h.gates = append(h.gates, g)
	h.log = append(h.log, "gate")
}

func TestRunHoldsAGatedTurnBackBeforeItsCallIsTaken(t *testing.T) {
	dir := t.TempDir()
	h := &gateHost{host: host{sessionLog: filepath.Join(dir, "session")}}
	script := filepath.Join(dir, "s.sh")
	body := `n=$(grep -c '^output' "$A10N_MOCK_SESSION_FILE" 2>/dev/null); n=${n:-0}
if [ "$n" = 0 ]; then
  printf '%s\n' '{"gate":{"ended":[1]},"type":"assistant","message":{"content":[{"type":"tool_use","id":"c0","name":"Bash","input":{"n":0}}]}}'
  exit 0
fi
printf '%s\n' '{"type":"result","result":"done"}'`
	if err := os.WriteFile(script, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(context.Background(), h, Params{Script: script, Dir: dir, Environ: []string{"PATH=/usr/bin:/bin"}, Prompt: "go"}); err != nil {
		t.Fatal(err)
	}
	if len(h.gates) != 1 || len(h.gates[0].Ended) != 1 || h.gates[0].Ended[0] != 1 {
		t.Fatalf("gates = %+v", h.gates)
	}
	joined := strings.Join(h.log, " ")
	if strings.Index(joined, "gate") > strings.Index(joined, "tool:Bash") {
		t.Fatalf("the gate was held after the call was taken: %s", joined)
	}
}

// noticeHost has something to tell the agent of from the moment the first call is
// carried out.
type noticeHost struct {
	host
	pending []string
}

func (h *noticeHost) Notice(bool) []string {
	if len(h.pending) == 0 {
		return nil
	}
	text := h.pending[0]
	h.pending = h.pending[1:]
	return []string{text}
}
func (h *noticeHost) Told(text string) { h.log = append(h.log, "told:"+text) }

// The agent is told of what ended after the last call of a script of the model, not
// between the calls one script made (More), and when its turn would end it is told
// instead of the turn ending: no end-of-turn hook runs for that, and it is not a block.
func TestRunTellsTheAgentWhatEndedAfterAScriptsLastCallAndAtTheTurnsEnd(t *testing.T) {
	h := &noticeHost{pending: []string{"A", "B", "C"}}
	body := `n=$(grep -c '^output' "$A10N_MOCK_SESSION_FILE" 2>/dev/null); n=${n:-0}
case "$n" in
0) printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"tool_use","id":"c0","name":"Bash","input":{}, "more":true}]}}';;
1) printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"tool_use","id":"c1","name":"Bash","input":{}}]}}';;
*) printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"text","text":"done"}]}}' '{"type":"result","result":"done"}';;
esac`
	dir := t.TempDir()
	h.host.sessionLog = filepath.Join(dir, "session")
	script := filepath.Join(dir, "s.sh")
	if err := os.WriteFile(script, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(context.Background(), h, Params{Script: script, Dir: dir, Environ: []string{"PATH=/usr/bin:/bin"}, Prompt: "go"}); err != nil {
		t.Fatal(err)
	}
	want := []string{"prompt", "tool:Bash", "tool:Bash", "told:A", "say:done", "told:B", "say:done", "told:C", "say:done", "stop:done:false"}
	if !reflect.DeepEqual(h.log, want) {
		t.Fatalf("log = %v, want %v", h.log, want)
	}
}
