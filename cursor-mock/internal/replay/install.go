package replay

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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
	files := map[string]string{
		filepath.Join(repo, ".cursor", "hooks.json"): rec.Setup["hooks.json"],
		filepath.Join(home, ".cursor", "hooks.json"): rec.Setup["user-hooks.json"],
	}
	modes := map[string]os.FileMode{}
	for name, body := range rec.Setup {
		if strings.HasSuffix(name, ".sh") {
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
	return l, nil
}

// concurrent is the hook log's canonical lines with each group of concurrent
// hooks put in a fixed order: the hooks of one event run side by side, so the
// order they log in is not the behaviour, while the order of the events is. A
// group is the run of consecutive lines about the same event (the payloads and
// what the hook scripts logged for it); sorted is only within it.
func concurrent(objs []map[string]any, lines []string) []string {
	out := append([]string(nil), lines...)
	for i := 0; i < len(objs); {
		j := i + 1
		for j < len(objs) && eventOf(objs[j]) == eventOf(objs[i]) {
			j++
		}
		sort.Strings(out[i:j])
		i = j
	}
	return out
}

func str(m map[string]any, k string) string { s, _ := m[k].(string); return s }

// eventOf is the hook event a logged line is about: a payload's own, or the one
// a hook script logged it for (at the top of the line, or in its hook_result).
func eventOf(o map[string]any) string {
	if e := str(o, "hook_event_name"); e != "" {
		return e
	}
	if e := str(o, "event"); e != "" {
		return e
	}
	r, _ := o["hook_result"].(map[string]any)
	return str(r, "event")
}
