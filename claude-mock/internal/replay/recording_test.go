package replay

import (
	"path/filepath"
	"testing"

	core "github.com/sloprail/harness-mocks/internal/replay"
)

// A recording is read into the calls the model made: bashfail's model ran two
// Bash commands and then said DONE.
func TestLoadReadsTheModelsCalls(t *testing.T) {
	rec, err := Adapter{}.Load(filepath.Join("..", "..", "snapshots", "runs", "bashfail"))
	if err != nil {
		t.Fatal(err)
	}
	if len(rec.Agent.Calls) != 2 || rec.Agent.Final != "DONE" {
		t.Fatalf("calls %d, final %q", len(rec.Agent.Calls), rec.Agent.Final)
	}
	if c := rec.Agent.Calls[1]; c.Tool != core.ToolShell || c.Input["command"] != "true" {
		t.Fatalf("second call %+v", c)
	}
}

// A sub-agent's turns are attached to the call that started it.
func TestLoadAttachesTheSubagent(t *testing.T) {
	rec, err := Adapter{}.Load(filepath.Join("..", "..", "snapshots", "runs", "isolated-worktree"))
	if err != nil {
		t.Fatal(err)
	}
	c := rec.Agent.Calls[0]
	if c.Tool != core.ToolSpawn || c.Sub == nil || len(c.Sub.Calls) != 3 {
		t.Fatalf("spawn %+v", c)
	}
}

// What the adapter does not map is refused, never guessed.
func TestLoadRefusesATool(t *testing.T) {
	_, err := Adapter{}.Load(filepath.Join("..", "..", "snapshots", "runs", "tool-errors"))
	if _, ok := err.(*Unbuildable); !ok {
		t.Fatalf("err = %v, want an Unbuildable", err)
	}
}
