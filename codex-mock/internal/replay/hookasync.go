package replay

import "encoding/json"

// asyncEvents are the events whose hooks the config marks async (SessionEnd's run synchronously all
// the same, and are not named).
func asyncEvents(hooksJSON string) map[string]bool {
	var f struct {
		Hooks map[string][]struct {
			Hooks []struct {
				Async bool `json:"async"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	out := map[string]bool{}
	if json.Unmarshal([]byte(hooksJSON), &f) != nil {
		return out
	}
	for ev, groups := range f.Hooks {
		for _, g := range groups {
			for _, h := range g.Hooks {
				if h.Async && ev != "SessionEnd" {
					out[ev] = true
				}
			}
		}
	}
	return out
}
