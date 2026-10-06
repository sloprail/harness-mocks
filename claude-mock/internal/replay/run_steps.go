package replay

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/sloprail/harness-mocks/internal/procexec"
	core "github.com/sloprail/harness-mocks/internal/replay"
)

// runSteps runs the mock once for the recording's own prompt and once for each later run of claude
// (a resume), in the same repository and home, so a later run finds the session an earlier one
// left; it returns what they streamed, one after the other as the recording's stream is.
func runSteps(ctx context.Context, mock string, s Scenario, rec core.Recording, work, repo string, env []string) (string, error) {
	base := append([]string{mock, "-p", "--model", "haiku", "--dangerously-skip-permissions", "--output-format", "stream-json", "--verbose"}, mockFlags(work)...)
	type invocation struct {
		script string
		args   []string
		prompt string
		step   ScenarioStep // the directory it runs in, and what is made there first
	}
	first := strings.Fields(rec.Setup["args"])
	if !hasFlag(first, "--session-id") && !resumes(first) {
		first = append([]string{"--session-id", sessionID}, first...)
	}
	main := sessionID // what a later run's <SESSION> is: the id the first run started under
	if id := rec.Setup["session"]; id != "" {
		main = id
	}
	runs := []invocation{{filepath.Join(work, "main.sh"), first, s.Prompt,
		ScenarioStep{Cwd: rec.Setup["cwd"], Symlink: rec.Setup["symlink"], Settings: s.Settings, Hook: s.Hook}}}
	for i, st := range s.Then {
		var args []string
		for _, a := range st.Args {
			if a == "<SESSION>" {
				a = main
			}
			args = append(args, a)
		}
		script := filepath.Join(work, fmt.Sprintf("step%d.sh", i+1))
		if err := os.WriteFile(script, []byte(strings.NewReplacer("<RUN>", repo, "\\u003cRUN\\u003e", repo).Replace(st.Script)), 0o755); err != nil {
			return "", err
		}
		runs = append(runs, invocation{script, args, st.Prompt, st})
	}
	var out strings.Builder
	for _, r := range runs {
		dir, err := prepareDir(repo, r.step)
		if err != nil {
			return "", err
		}
		// @TRANSCRIPTS@ is where this run's directory keeps its sessions' transcripts (the capture's own placeholder)
		transcripts := filepath.Join(work, "home", ".claude", "projects", regexp.MustCompile(`[^A-Za-z0-9]`).ReplaceAllString(dir, "-"))
		args := make([]string, len(r.args))
		for i, a := range r.args {
			args[i] = strings.ReplaceAll(a, "@TRANSCRIPTS@", transcripts)
		}
		argv := append(append(append([]string{}, base...), "--script", r.script), append(args, r.prompt)...)
		res, err := procexec.Run(ctx, procexec.Spec{Argv: argv, Dir: dir, Env: env, Timeout: 3 * time.Minute})
		if err != nil || res.ExitCode != wantExit(rec) || res.TimedOut {
			return "", &core.MockFailure{Detail: fmt.Sprintf("%v (exit %d): %s", err, res.ExitCode, res.Stderr)}
		}
		out.Write(res.Stdout)
	}
	return out.String(), nil
}

// mockFlags are the dirs the replay's mock is given: where claude keeps its config and its plugins
// (under the home a capture gives it, by default, so they are flags here, not environment).
func mockFlags(work string) []string {
	return []string{"--config-dir", filepath.Join(work, "home", ".claude"), "--plugin-cache-dir", filepath.Join(work, "plugins")}
}

// prepareDir is the directory a run starts in, made as a capture makes it: the symlink first
// ("<name> <target>": <name> of the repository links to <target>, also of it), then the directory (a
// symlink is not made one), then the project files that directory has of its own.
func prepareDir(repo string, st ScenarioStep) (string, error) {
	if st.Symlink != "" {
		f := strings.Fields(st.Symlink)
		if len(f) != 2 {
			return "", fmt.Errorf("the symlink %q is not \"<name> <target>\"", st.Symlink)
		}
		if err := os.MkdirAll(filepath.Join(repo, f[1]), 0o755); err != nil {
			return "", err
		}
		_ = os.Remove(filepath.Join(repo, f[0]))
		if err := os.Symlink(filepath.Join(repo, f[1]), filepath.Join(repo, f[0])); err != nil {
			return "", err
		}
	}
	dir := repo
	if st.Cwd != "" {
		dir = filepath.Join(repo, st.Cwd)
		if fi, err := os.Lstat(dir); err != nil || fi.Mode()&os.ModeSymlink == 0 {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return "", err
			}
		}
	}
	if st.Settings != "" {
		if err := os.MkdirAll(filepath.Join(dir, ".claude"), 0o755); err != nil {
			return "", err
		}
		if err := os.WriteFile(filepath.Join(dir, ".claude", "settings.json"), []byte(st.Settings), 0o644); err != nil {
			return "", err
		}
	}
	if st.Hook != "" {
		if err := os.WriteFile(filepath.Join(dir, "hook.sh"), []byte(st.Hook), 0o755); err != nil {
			return "", err
		}
	}
	return dir, nil
}
