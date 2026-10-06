package replay

import (
	"fmt"
	"path/filepath"
	"strings"

	core "github.com/sloprail/harness-mocks/internal/replay"
)

// ephemeralFlag is the run option for a session nothing of which is kept.
const ephemeralFlag = "--ephemeral"

// ephemeral is whether the run was made with --ephemeral: it leaves no rollout.
func ephemeral(setup string) bool {
	return strings.Contains(" "+strings.Join(strings.Fields(readFile(filepath.Join(setup, "args"))), " ")+" ", " "+ephemeralFlag+" ")
}

// loadEphemeral reads a run that kept no rollout. What the model did is only what the event stream
// shows: each command it ran, in order, and its last message; so only a plain run is read, one that
// ran commands and answered once. The command is what the stream shows of it, and the options the
// model gave the tool are not shown: the mock runs it with none.
func loadEphemeral(runDir, setup, sample string, cmdline []string) (core.Recording, error) {
	stream, err := readJSONL(filepath.Join(sample, "stream.jsonl"))
	if err != nil {
		return core.Recording{}, err
	}
	var calls []core.Call
	final, answers := "", 0
	for _, l := range stream {
		item, _ := l["item"].(map[string]any)
		switch {
		case l["type"] == "item.started" && item["type"] == "command_execution":
			calls = append(calls, core.Call{Tool: core.ToolShell, Input: map[string]any{"command": shellInner(fmt.Sprint(item["command"]))}})
		case l["type"] == "item.completed" && item["type"] == "agent_message":
			final, answers = fmt.Sprint(item["text"]), answers+1
		case l["type"] == "item.completed" && (item["type"] == "command_execution" || item["type"] == "error"):
		case l["type"] == "item.started" || l["type"] == "item.completed":
			return core.Recording{}, unbuildable(fmt.Errorf("an ephemeral run whose stream has a %v item: only commands and one answer are read", item["type"]))
		}
	}
	if answers != 1 {
		return core.Recording{}, unbuildable(fmt.Errorf("an ephemeral run with %d answers: only one is read", answers))
	}
	return core.Recording{
		Dir:    runDir,
		Prompt: strings.TrimSpace(readFile(filepath.Join(setup, "prompt.txt"))),
		Setup:  setupOf(setup, sample, cmdline),
		Agent:  core.Agent{Calls: calls, Final: final},
	}, nil
}
