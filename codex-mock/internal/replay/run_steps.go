package replay

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sloprail/harness-mocks/internal/procexec"
	core "github.com/sloprail/harness-mocks/internal/replay"
)

// runSteps runs the mock once for the recording's own prompt and once for each later run of the
// harness (a resume, a fork), against the same CODEX_HOME, and returns what they printed, one after
// the other as the recording's stream is. A later run's "<SESSION>" is the first run's session.
func runSteps(ctx context.Context, mock string, s Scenario, repo, root, scriptsAt string, env []string) (string, error) {
	type invocation struct {
		script, prompt, cwd string
		args                []string
	}
	runs := []invocation{{script: s.Script, prompt: s.Prompt, cwd: repo}}
	for _, t := range s.Then {
		cwd := repo
		if t.Cwd != "" { // a directory beside the repository, empty
			cwd = filepath.Join(root, t.Cwd)
			if err := os.MkdirAll(cwd, 0o755); err != nil {
				return "", err
			}
		}
		runs = append(runs, invocation{script: t.Script, prompt: t.Prompt, cwd: cwd, args: t.Args})
	}
	var stdout strings.Builder
	for i, r := range runs {
		script := filepath.Join(root, fmt.Sprintf("scenario%d.sh", i))
		if err := os.WriteFile(script, []byte(scriptText(r.script, scriptsAt, repo)), 0o755); err != nil {
			return "", err
		}
		argv := []string{mock, "exec", "--dangerously-bypass-hook-trust", "--json", "--skip-git-repo-check", "--script", script, "-m", "mock-model"}
		for _, a := range r.args {
			if a == "<SESSION>" {
				a = firstThread(stdout.String())
			}
			argv = append(argv, a)
		}
		res, err := procexec.Run(ctx, procexec.Spec{Argv: append(argv, r.prompt), Dir: r.cwd, Env: env})
		if err != nil || res.ExitCode != 0 {
			return "", &core.MockFailure{Detail: fmt.Sprintf("run %d: %v (exit %d): %s", i, err, res.ExitCode, res.Stderr)}
		}
		stdout.Write(res.Stdout)
	}
	return stdout.String(), nil
}

// firstThread is the session the first run's stream starts.
func firstThread(stream string) string {
	lines, _ := parseJSONL(stream)
	for _, id := range threadsOf(lines) {
		return id
	}
	return ""
}
