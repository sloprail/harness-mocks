package replay

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// trustHooks trusts, in the run's config.toml, the hooks of the sources the recorded run trusted
// (capture.sh: setup/trust-hooks), at the key and hash codex gave them in the sample's hooks-list.json
// (its <TMP> and <RUN> are this replay's root and repository).
func trustHooks(home string, s Scenario, root, repo string) error {
	if strings.TrimSpace(s.TrustHooks) == "" {
		return nil
	}
	var list []struct {
		Key, Source, CurrentHash string
	}
	if err := json.Unmarshal([]byte(s.HooksList), &list); err != nil {
		return fmt.Errorf("the sample's hooks-list.json: %w", err)
	}
	trusted := map[string]bool{}
	for _, src := range strings.Fields(s.TrustHooks) {
		trusted[src] = true
	}
	f, err := os.OpenFile(filepath.Join(home, "config.toml"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	for _, h := range list {
		if trusted[h.Source] {
			key := strings.NewReplacer("<TMP>", root, "<RUN>", repo).Replace(h.Key)
			fmt.Fprintf(f, "[hooks.state.%q]\ntrusted_hash = %q\n\n", key, h.CurrentHash)
		}
	}
	return nil
}
