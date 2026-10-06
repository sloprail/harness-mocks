package replay

import (
	"os"
	"path/filepath"
	"strings"
)

// pluginShim is a directory holding a `claude` for a scenario's prepare.sh to run: it accepts the
// `claude plugin ...` commands that install a plugin from a local marketplace and refuses
// everything else. The capture installs a plugin into claude's own home; the mock reads a local
// marketplace named in the settings itself (extraKnownMarketplaces), so installing changes nothing
// it reads, and the replay needs no claude on the machine (CI has none).
func pluginShim(work string) (string, error) {
	dir := filepath.Join(work, "shims")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	body := "#!/bin/sh\n[ \"$1\" = plugin ] && exit 0\necho \"claude: only 'plugin' commands are available to a replayed scenario's preparation\" >&2\nexit 1\n"
	return dir, os.WriteFile(filepath.Join(dir, "claude"), []byte(body), 0o755)
}

// withShimsFirst is env with the shims directory ahead of the PATH it holds.
func withShimsFirst(env []string, shims string) []string {
	out := make([]string, 0, len(env)+1)
	for _, kv := range env {
		if !strings.HasPrefix(kv, "PATH=") {
			out = append(out, kv)
			continue
		}
		out = append(out, "PATH="+shims+string(os.PathListSeparator)+strings.TrimPrefix(kv, "PATH="))
	}
	return out
}
