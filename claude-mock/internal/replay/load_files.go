package replay

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"
)

func readRun(path string) (run, error) {
	var r run
	b, err := os.ReadFile(path)
	if err != nil {
		return r, fmt.Errorf("run.yaml: %w", err)
	}
	if err := yaml.Unmarshal(b, &r); err != nil {
		return r, fmt.Errorf("run.yaml: %w", err)
	}
	return r, nil
}

// subagentTurns are the turns of every sub-agent a run recorded, by the id of
// the tool call that started it. Each sub-agent is a transcript file with a
// .meta.json beside it that names that call.
func subagentTurns(dir string) (map[string]turns, error) {
	metas, _ := filepath.Glob(filepath.Join(dir, "*.meta.json"))
	out := map[string]turns{}
	for _, m := range metas {
		var meta struct {
			ToolUseID string `json:"toolUseId"`
		}
		if err := json.Unmarshal([]byte(readFile(m)), &meta); err != nil || meta.ToolUseID == "" {
			return nil, unbuildable(fmt.Errorf("%s does not name the call that started the sub-agent", m))
		}
		records, err := readJSONL(strings.TrimSuffix(m, ".meta.json") + ".jsonl")
		if err != nil {
			return nil, err
		}
		sub, err := modelTurns(records)
		if err != nil {
			return nil, unbuildable(fmt.Errorf("sub-agent: %w", err))
		}
		out[meta.ToolUseID] = sub
	}
	return out, nil
}
