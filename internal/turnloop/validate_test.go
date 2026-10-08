package turnloop

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sloprail/harness-mocks/internal/toolspec"
)

// validatingHost is a host whose script's calls are checked.
type validatingHost struct {
	host
	refusals toolspec.Refusals
}

func (h *validatingHost) Tools() toolspec.Schema {
	return toolspec.Schema{Harness: "test", Tools: []toolspec.Tool{{Name: "Bash", Params: []toolspec.Param{{Name: "n", Type: toolspec.Integer}}}}}
}
func (h *validatingHost) Refusals() *toolspec.Refusals { return &h.refusals }

func runValidating(t *testing.T, h *validatingHost, body string) error {
	t.Helper()
	dir := t.TempDir()
	h.sessionLog = filepath.Join(dir, "session")
	script := filepath.Join(dir, "s.sh")
	if err := os.WriteFile(script, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Run(context.Background(), h, Params{Script: script, Dir: dir, Environ: []string{"PATH=/usr/bin:/bin"}, Prompt: "go"})
	return err
}

// A call the schema refuses ends the run with the refusal, before the host is
// given the call, and the run keeps it.
func TestRunRefusesAScriptCallTheSchemaDoesNotAllow(t *testing.T) {
	h := &validatingHost{}
	err := runValidating(t, h, `printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"tool_use","id":"x","name":"Teleport","input":{}}]}}'`)
	if err == nil || !strings.Contains(err.Error(), "Teleport") {
		t.Fatalf("err = %v, want a refusal naming the tool", err)
	}
	if h.refusals.Err() == nil {
		t.Fatal("the refusal is not kept for the run")
	}
	for _, line := range h.log {
		if strings.HasPrefix(line, "tool:") {
			t.Fatalf("the refused call was played: %v", h.log)
		}
	}
}

// A refusal another agent of the run already earned ends this one's next turn.
func TestRunEndsWithARefusalAnotherAgentEarned(t *testing.T) {
	h := &validatingHost{}
	h.refusals.Set(&toolspec.Error{Harness: "test", Issues: []toolspec.Issue{{Kind: toolspec.UnknownTool, Tool: "Teleport"}}})
	err := runValidating(t, h, `printf '%s\n' '{"type":"result","result":"done"}'`)
	if err == nil || !strings.Contains(err.Error(), "Teleport") {
		t.Fatalf("err = %v", err)
	}
}

// Only the refusal of a call is kept: another error of a script is its agent's own.
func TestRefusalsKeepOnlyARefusedCall(t *testing.T) {
	var r toolspec.Refusals
	r.Set(os.ErrNotExist)
	if r.Err() != nil {
		t.Fatal("an error that is not a refused call was kept")
	}
}
