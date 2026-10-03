package hooks

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// AsyncSessionEndFiles are the hooks files, in load order, that mark a
// SessionEnd command handler async. Codex runs such a hook synchronously
// all the same, and warns of it once per handler, naming the file (recorded:
// runs/session-end-hook-failure).
func AsyncSessionEndFiles(codexHome, cwd string) []string {
	var files []string
	for _, dir := range []string{codexHome, filepath.Join(cwd, ".codex")} {
		if dir == "" {
			continue
		}
		path := filepath.Join(dir, "hooks.json")
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var f struct {
			Hooks map[Event][]struct {
				Hooks []struct {
					Type  string `json:"type"`
					Async bool   `json:"async"`
				} `json:"hooks"`
			} `json:"hooks"`
		}
		if json.Unmarshal(data, &f) != nil {
			continue
		}
		for _, g := range f.Hooks[SessionEnd] {
			for _, h := range g.Hooks {
				if h.Type == "command" && h.Async {
					files = append(files, path)
				}
			}
		}
	}
	return files
}
