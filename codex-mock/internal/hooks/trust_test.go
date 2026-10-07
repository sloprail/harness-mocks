package hooks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// listed is an entry of the hooks/list answer codex gave a recorded run (its sample's hooks-list.json).
type listed struct {
	Key, EventName, Matcher, Command, CurrentHash, Source string
	TimeoutSec                                            int
	Async                                                 bool
	StatusMessage                                         string
}

// recordedList reads the hooks codex listed in a recorded run, and the run's setup directory.
func recordedList(t *testing.T, run string) (list []listed, setup string) {
	t.Helper()
	dir := filepath.Join("..", "..", "snapshots", "runs", run)
	samples, _ := filepath.Glob(filepath.Join(dir, "samples", "*", "hooks-list.json"))
	if len(samples) == 0 {
		t.Fatalf("%s has no recorded hooks-list.json", run)
	}
	data, err := os.ReadFile(samples[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &list); err != nil {
		t.Fatal(err)
	}
	return list, filepath.Join(dir, "setup")
}

// The hash of every hook codex listed in the recorded runs is the one hookHash computes: the user and
// project hooks (runs/hook-trust-listed), the hooks.json of runs/hook-hashes with a timeout, a status
// message, an async hook, a SessionStart with and without a matcher, a matcher on Stop, UserPromptSubmit
// and Interrupt (which the event ignores), and a plugin's hook (runs/plugin-hook-trusted).
func TestHookHashIsCodexs(t *testing.T) {
	for _, run := range []string{"hook-trust-listed", "plugin-hook-trusted", "plugin-hook-untrusted", "hook-hashes"} {
		list, setup := recordedList(t, run)
		var file struct {
			Hooks map[Event][]fileGroup `json:"hooks"`
		}
		if data, err := os.ReadFile(filepath.Join(setup, "hooks.json")); err == nil && run == "hook-hashes" {
			if err := json.Unmarshal(data, &file); err != nil {
				t.Fatal(err)
			}
		}
		for _, h := range list {
			ev, matcher, handler := Event(""), h.Matcher, fileHandler{Type: "command", Command: h.Command, Timeout: h.TimeoutSec, Async: h.Async, StatusMessage: h.StatusMessage}
			parts := strings.Split(h.Key, ":")
			n := len(parts)
			group, _ := strconv.Atoi(parts[n-2])
			index, _ := strconv.Atoi(parts[n-1])
			for e := range file.Hooks { // the hooks.json the run was given: its matcher, not the one codex reports
				if snake(e) == parts[n-3] {
					ev = e
					matcher = file.Hooks[e][group].Matcher
					handler = file.Hooks[e][group].Hooks[index]
				}
			}
			if ev == "" {
				for _, e := range []Event{SessionStart, UserPromptSubmit, PreToolUse, PostToolUse, Stop, Interrupt} {
					if snake(e) == parts[n-3] {
						ev = e
					}
				}
			}
			if got := hookHash(ev, matcher, handler); got != h.CurrentHash {
				t.Errorf("%s %s: hash %s, codex's %s", run, h.Key, got, h.CurrentHash)
			}
		}
	}
}
