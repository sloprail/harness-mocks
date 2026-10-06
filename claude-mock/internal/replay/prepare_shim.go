package replay

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// prepareShim is a directory holding a `claude` for a scenario's prepare.sh to run. It accepts the
// `claude plugin ...` commands that install a plugin from a local marketplace: the capture installs a
// plugin into claude's own home, the mock reads a local marketplace named in the settings itself
// (extraKnownMarketplaces), so installing changes nothing it reads, and the replay needs no claude on
// the machine (CI has none). A `claude -p ... '<prompt>'` is an earlier run of the recording: the
// shim runs the mock on the script of the recorded run with that prompt (mock is its command line,
// with the replay's own flags), and refuses a prompt it has no recorded run for. Anything else
// is refused.
func prepareShim(work string, mock []string, earlier []ScenarioEarlier, scripts string) (string, error) {
	dir := filepath.Join(work, "shims")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	var cases strings.Builder
	for _, e := range earlier {
		fmt.Fprintf(&cases, "  %s) script=%s ;;\n", shellQuote(e.Prompt), shellQuote(filepath.Join(scripts, e.Script)))
	}
	quoted := make([]string, len(mock))
	for i, w := range mock {
		quoted[i] = shellQuote(w)
	}
	body := `#!/bin/sh
[ "$1" = plugin ] && exit 0
if [ "$1" = -p ]; then
  for last; do :; done
  case "$last" in
` + cases.String() + `  *) echo "claude: no recorded run for the prompt: $last" >&2; exit 1 ;;
  esac
  exec ` + strings.Join(quoted, " ") + ` "$@" --script "$script"
fi
echo "claude: only 'plugin' commands and the recording's earlier runs are available to a replayed scenario's preparation" >&2
exit 1
`
	return dir, os.WriteFile(filepath.Join(dir, "claude"), []byte(body), 0o755)
}

// shellQuote is s as one word of a shell command.
func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

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
