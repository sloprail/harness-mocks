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

// keyOrder is the field of a hook payload that carries the order its keys were
// written in: the order is what a hook command that reads the raw text sees,
// and the unified canonicalisation sorts keys.
const keyOrder = "_key_order"

// readJSONL parses the recording's file at path as JSON objects, one per line
// (blank lines aside). A line that is not a JSON object is an error naming the
// file and line, never skipped: a replay that ignored a line of its recording
// would compare less than the recording holds. An absent file has no lines.
func readJSONL(path string) ([]map[string]any, error) {
	out, err := parseJSONL(readFile(path), false)
	if err != nil {
		return nil, unbuildable(fmt.Errorf("%s: %w", path, err))
	}
	return out, nil
}

// parseJSONL parses text as JSON objects, one per line. With ordered, each
// object also carries the order of its own (top-level) keys under keyOrder.
func parseJSONL(text string, ordered bool) ([]map[string]any, error) {
	var out []map[string]any
	for i, l := range strings.Split(text, "\n") {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(l), &m); err != nil || m == nil {
			return nil, fmt.Errorf("line %d is not a JSON object: %.80s", i+1, l)
		}
		if ordered {
			keys, err := topKeys(l)
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", i+1, err)
			}
			m[keyOrder] = keys
		}
		out = append(out, m)
	}
	return out, nil
}

// topKeys are the keys of the JSON object in text, in the order written.
func topKeys(text string) ([]any, error) {
	dec := json.NewDecoder(strings.NewReader(text))
	if _, err := dec.Token(); err != nil { // the opening brace
		return nil, err
	}
	var keys []any
	for dec.More() {
		k, err := dec.Token()
		if err != nil {
			return nil, err
		}
		keys = append(keys, k)
		var skip json.RawMessage
		if err := dec.Decode(&skip); err != nil {
			return nil, err
		}
	}
	return keys, nil
}
