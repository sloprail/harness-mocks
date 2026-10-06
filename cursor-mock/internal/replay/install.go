package replay

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/sloprail/harness-mocks/internal/procexec"
	core "github.com/sloprail/harness-mocks/internal/replay"
)

// layout is where a replay put the run's setup: the workspace, the directory the
// mock starts from (the workspace itself, or a symlink to it), the scenario
// script, the hook log and the mock's environment.
type layout struct {
	repo, cwd, main, hookLog string
	env                      []string
	// later are the steps after the first, each with its script, command line and
	// directory (install.go, steps.go).
	later []laterStep
}

// laterStep is how a later step of a run is started.
type laterStep struct {
	script, prompt, dir string
	flags               []string // the step's setup/args, with <SESSION> for the first step's session
}

// install lays the run's setup out under work as a capture lays out its own: a
// git repository with the project's hooks in .cursor/ (the scripts in
// .cursor/hooks/), the user's hooks in the home's .cursor/, the temp root, and
// the scenario the mock plays in place of the model.
func (a Adapter) install(work string, rec core.Recording) (layout, error) {
	repo, home, tmp, scripts := filepath.Join(work, "repo"), filepath.Join(work, "home"), filepath.Join(work, "tmp"), filepath.Join(work, "scripts")
	l := layout{repo: repo, cwd: repo, main: filepath.Join(work, "main.sh"), hookLog: filepath.Join(work, "hook.log")}
	for _, d := range []string{filepath.Join(repo, ".cursor", "hooks"), filepath.Join(home, ".cursor"), scripts} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return l, err
		}
	}
	if err := os.Mkdir(tmp, 0o700); err != nil {
		return l, err
	}
	l.env = a.baseEnv(home, tmp, l.hookLog)
	ctx := context.Background()
	for _, argv := range [][]string{
		{"git", "-C", repo, "init", "-q", "-b", "main"},
		{"git", "-C", repo, "-c", "commit.gpgsign=false", "commit", "-q", "--allow-empty", "-m", "init"},
	} {
		if res, err := procexec.Run(ctx, procexec.Spec{Argv: argv, Env: l.env}); err != nil || res.ExitCode != 0 {
			return l, fmt.Errorf("%s: %v %s", strings.Join(argv, " "), err, res.Stderr)
		}
	}
	if prep := rec.Setup["prepare.sh"]; prep != "" { // as the capture runs it: in the repository, under the run's home
		path := filepath.Join(work, "prepare.sh")
		if err := os.WriteFile(path, []byte(prep), 0o755); err != nil {
			return l, err
		}
		if res, err := procexec.Run(ctx, procexec.Spec{Argv: []string{"sh", path}, Dir: repo, Env: l.env}); err != nil || res.ExitCode != 0 {
			return l, fmt.Errorf("the setup's prepare.sh failed: %v %s", err, res.Stderr)
		}
	}
	files := map[string]string{
		filepath.Join(repo, ".cursor", "hooks.json"): rec.Setup["hooks.json"],
		filepath.Join(home, ".cursor", "hooks.json"): rec.Setup["user-hooks.json"],
	}
	modes := map[string]os.FileMode{}
	for name, body := range rec.Setup {
		if rel, ok := strings.CutPrefix(name, homeFilePrefix); ok {
			if err := os.MkdirAll(filepath.Dir(filepath.Join(home, rel)), 0o755); err != nil {
				return l, err
			}
			files[filepath.Join(home, rel)] = body
			continue
		}
		if strings.HasSuffix(name, ".sh") && name != "prepare.sh" {
			path := filepath.Join(repo, ".cursor", "hooks", name)
			files[path], modes[path] = body, 0o755
		}
	}
	s := Denormalize(rec, scripts, &Paths{Run: repo, Tmp: work, RunDirname: encode(repo)})
	files[l.main], modes[l.main] = s.Script, 0o755
	for name, body := range s.Scripts {
		path := filepath.Join(scripts, name)
		files[path], modes[path] = body, 0o755
	}
	lines := map[string]int{repo: s.Lines} // the records each directory's transcript holds after the steps so far
	for i, name := range stepNames(rec.Setup) {
		st := rec.Then[i]
		flags, err := flagWords(rec.Setup["then-"+name+"-args"], true)
		if err != nil {
			return l, err
		}
		dir := stepDir(work, repo, rec.Setup, name)
		after := 0
		if contains(flags, "--resume") { // a resumed session's file holds what the steps before it said, there
			after = lines[dir]
		}
		sc := DenormalizeStep(st, i+1, scripts, &Paths{Run: repo, Tmp: work, RunDirname: encode(repo)}, after)
		lines[dir] += sc.Lines
		step := laterStep{script: filepath.Join(work, fmt.Sprintf("main%d.sh", i+1)), prompt: st.Prompt, dir: dir}
		files[step.script], modes[step.script] = sc.Script, 0o755
		for sname, body := range sc.Scripts {
			path := filepath.Join(scripts, sname)
			files[path], modes[path] = body, 0o755
		}
		step.flags = flags
		l.later = append(l.later, step)
	}
	for path, body := range files {
		if body == "" && filepath.Base(path) == "hooks.json" && strings.HasPrefix(path, home) {
			continue // no user source
		}
		mode := modes[path]
		if mode == 0 {
			mode = 0o644
		}
		if err := os.WriteFile(path, []byte(body), mode); err != nil {
			return l, err
		}
	}
	// the run's own environment, on top of the hermetic one
	for _, line := range strings.Split(rec.Setup["env"], "\n") {
		if line != "" {
			l.env = append(l.env, line)
		}
	}
	if name := strings.TrimSpace(rec.Setup["symlink"]); name != "" { // the run starts from a symlink to the repository
		l.cwd = filepath.Join(work, name)
		if err := os.Symlink(repo, l.cwd); err != nil {
			return l, err
		}
	}
	for _, st := range l.later { // a step's own directory has the project's hooks, as the capture makes it
		if st.dir != repo {
			if err := copyDir(filepath.Join(repo, ".cursor"), filepath.Join(st.dir, ".cursor")); err != nil {
				return l, err
			}
		}
	}
	return l, nil
}

// copyDir copies a directory tree (the project's .cursor) to dst.
func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, b, info.Mode())
	})
}

func contains(words []string, w string) bool {
	for _, x := range words {
		if x == w {
			return true
		}
	}
	return false
}
