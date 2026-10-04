package replay

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Recording is one recorded run: its folder (run.yaml, setup/, samples/), and
// the latest sample of it.
type Recording struct {
	Dir, Setup, Sample string
}

// Load is the recording in runDir; a run with no sample has an empty Sample.
func Load(runDir string) (Recording, error) {
	if fi, err := os.Stat(runDir); err != nil || !fi.IsDir() {
		return Recording{}, fmt.Errorf("%s is not a recorded run", runDir)
	}
	rec := Recording{Dir: runDir, Setup: filepath.Join(runDir, "setup")}
	samples, _ := filepath.Glob(filepath.Join(runDir, "samples", "*"))
	if len(samples) > 0 {
		rec.Sample = samples[len(samples)-1]
	}
	return rec, nil
}

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
