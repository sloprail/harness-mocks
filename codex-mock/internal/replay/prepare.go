package replay

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sloprail/harness-mocks/internal/procexec"
)

// codexStub stands for the `codex plugin` commands a recorded run's prepare.sh used to set up the run's
// home: it writes into config.toml what the real ones wrote (recorded: runs/plugin-hooks, read by the
// mock: a marketplace declared with its local source, a plugin enabled). Nothing else of codex is
// stubbed: another command is an error, never ignored.
const codexStub = `#!/bin/sh
[ "$1" = plugin ] || { echo "codex stub: only plugin commands" >&2; exit 2; }
shift
cfg="$CODEX_HOME/config.toml"
case "$1 $2" in
"marketplace add")
  name=$(jq -r .name "$3/.agents/plugins/marketplace.json")
  printf '[marketplaces.%s]\nsource_type = "local"\nsource = "%s"\n\n' "$name" "$3" >>"$cfg" ;;
"add "*)
  printf '[plugins."%s"]\nenabled = true\n\n' "$2" >>"$cfg" ;;
*) echo "codex stub: plugin $*" >&2; exit 2 ;;
esac
`

// sedStub makes the BSD form `sed -i ” ...` of a recorded script work where sed is GNU's.
const sedStub = `#!/bin/sh
real=/usr/bin/sed; [ -x "$real" ] || real=/bin/sed
if "$real" --version >/dev/null 2>&1 && [ "$1" = -i ] && [ "$2" = "" ]; then shift 2; set -- -i "$@"; fi
exec "$real" "$@"
`

// prepare runs the recorded run's prepare.sh in the scratch repository, with the stubs on PATH, as
// capture.sh ran it with the real codex: HOME and CODEX_HOME are the run's.
func prepare(ctx context.Context, script, root, repo, home string, env []string) error {
	stubs := filepath.Join(root, "stubs")
	if err := os.MkdirAll(stubs, 0o755); err != nil {
		return err
	}
	for name, body := range map[string]string{"codex": codexStub, "sed": sedStub, "prepare.sh": script} {
		if err := os.WriteFile(filepath.Join(stubs, name), []byte(body), 0o755); err != nil {
			return err
		}
	}
	res, err := procexec.Run(ctx, procexec.Spec{Argv: []string{"sh", filepath.Join(stubs, "prepare.sh")}, Dir: repo,
		Env: append(append([]string{}, env...), "PATH="+stubs+":"+pathIn(env), "HOME="+filepath.Join(root, "home"))})
	if err != nil || res.ExitCode != 0 {
		return fmt.Errorf("the recorded prepare.sh: %v (exit %d): %s", err, res.ExitCode, res.Stderr)
	}
	return nil
}

// pathIn is the PATH of an environment (the one the entrypoint read and passed down), "" when it has none.
func pathIn(env []string) string {
	for i := len(env) - 1; i >= 0; i-- {
		if v, ok := strings.CutPrefix(env[i], "PATH="); ok {
			return v
		}
	}
	return ""
}
