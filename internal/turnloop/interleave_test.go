package turnloop

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/sloprail/harness-mocks/internal/scenario"
)

// A turn of two calls, then (once both are recorded) the result.
const twoCalls = `n=$(grep -c '^output' "$A10N_MOCK_SESSION_FILE" 2>/dev/null); n=${n:-0}
if [ "$n" -lt 2 ]; then
  printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"tool_use","id":"a","name":"Bash","input":{}}]}}' \
    '{"type":"assistant","message":{"content":[{"type":"tool_use","id":"b","name":"Task","input":{}}]}}'
  exit 0
fi
printf '%s\n' '{"type":"result","result":"done"}'`

// interleavingHost starts all of a turn's calls before it completes any.
type interleavingHost struct{ host }

func (h *interleavingHost) Start(ctx context.Context, tu scenario.ToolUse) func() {
	h.log = append(h.log, "start:"+tu.Name)
	return func() { h.host.Tool(ctx, tu) }
}

func TestRunStartsAllTheCallsOfATurnBeforeCompletingAnyForAnInterleaver(t *testing.T) {
	dir := t.TempDir()
	h := &interleavingHost{host{sessionLog: filepath.Join(dir, "session")}}
	script := filepath.Join(dir, "s.sh")
	if err := os.WriteFile(script, []byte(twoCalls), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Run(context.Background(), h, Params{Tools: testTools, Script: script, Dir: dir, Environ: []string{"PATH=/usr/bin:/bin"}, Prompt: "go"})
	want := []string{"prompt", "start:Bash", "start:Task", "tool:Bash", "tool:Task", "stop::false"}
	if err != nil || !reflect.DeepEqual(h.log, want) {
		t.Fatalf("err=%v log=%v, want %v", err, h.log, want)
	}
}

func TestRunCarriesOutOnlyTheFirstCallOfATurnForAHostThatDoesNotInterleave(t *testing.T) {
	h := &host{}
	_, err := run(t, h, 0, twoCalls)
	// the second call is never made: the script asks again and its first call
	// runs again, as it did before a turn could have several
	want := []string{"prompt", "tool:Bash", "tool:Bash", "stop::false"}
	if err != nil || !reflect.DeepEqual(h.log, want) {
		t.Fatalf("err=%v log=%v, want %v", err, h.log, want)
	}
}
