package replay

import (
	"encoding/json"
	"os"
	"strings"
)

func readFile(path string) string {
	b, _ := os.ReadFile(path)
	return string(b)
}

// jsonLines parses text as JSON objects, one per line, skipping other lines.
func jsonLines(text string) []map[string]any {
	var out []map[string]any
	for _, l := range strings.Split(text, "\n") {
		var m map[string]any
		if json.Unmarshal([]byte(strings.TrimSpace(l)), &m) == nil && m != nil {
			out = append(out, m)
		}
	}
	return out
}
