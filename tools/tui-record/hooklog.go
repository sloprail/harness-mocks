package main

import (
	"bufio"
	"encoding/json"
	"os"
)

// hookKey is the field of a hook log line that names the event: every harness's payload has it.
const hookKey = "hook_event_name"

// countHooks is how many lines of the hook log (one JSON payload per line, as the hook script
// appends it) are of the event. A line that is not whole yet, being written, is not counted.
func countHooks(path, event string) int {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()
	n := 0
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<26)
	for sc.Scan() {
		var line map[string]any
		if json.Unmarshal(sc.Bytes(), &line) == nil && line[hookKey] == event {
			n++
		}
	}
	return n
}
