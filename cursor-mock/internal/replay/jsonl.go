package replay

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

func readFile(path string) string {
	b, _ := os.ReadFile(path)
	return string(b)
}

// readJSONL parses the recording's file at path as JSON objects, one per line
// (blank lines aside). A line that is not a JSON object is an error naming the
// file and line, never skipped: a replay that ignored a line of its recording
// would compare less than the recording holds. An absent file has no lines.
func readJSONL(path string) ([]map[string]any, error) {
	out, err := parseJSONL(readFile(path))
	if err != nil {
		return nil, &Unbuildable{Reason: fmt.Sprintf("%s: %v", path, err)}
	}
	return out, nil
}

// parseJSONL parses text as JSON objects, one per line (or several run together on one).
func parseJSONL(text string) ([]map[string]any, error) {
	var out []map[string]any
	for i, l := range strings.Split(text, "\n") {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(l), &m); err == nil && m != nil {
			out = append(out, m)
			continue
		}
		// hooks that run side by side append to one log, and what they write can
		// run together on a line: several objects, nothing else, is still the log
		dec := json.NewDecoder(strings.NewReader(l))
		var run []map[string]any
		for dec.More() {
			var o map[string]any
			if err := dec.Decode(&o); err != nil || o == nil {
				return nil, fmt.Errorf("line %d is not a JSON object: %.80s", i+1, l)
			}
			run = append(run, o)
		}
		if len(run) == 0 {
			return nil, fmt.Errorf("line %d is not a JSON object: %.80s", i+1, l)
		}
		out = append(out, run...)
	}
	return out, nil
}
