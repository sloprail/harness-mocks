package main

import (
	"bytes"
	"fmt"
	"os/exec"
	"path"
	"strings"
)

// repo runs git against one repository directory. Every read of source goes
// through git objects, never the working tree.
type repo struct{ dir string }

func (r repo) git(args ...string) ([]byte, error) {
	cmd := exec.Command("git", append([]string{"-C", r.dir}, args...)...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(errb.String()))
	}
	return out.Bytes(), nil
}

func (r repo) lines(sep string, args ...string) ([]string, error) {
	out, err := r.git(args...)
	if err != nil {
		return nil, err
	}
	var res []string
	for _, s := range strings.Split(string(out), sep) {
		if s != "" {
			res = append(res, s)
		}
	}
	return res, nil
}

// resolve turns a revision into a full commit id.
func (r repo) resolve(rev string) (string, error) {
	out, err := r.git("rev-parse", "--verify", "--quiet", rev+"^{commit}")
	return strings.TrimSpace(string(out)), err
}

// changed lists paths whose content differs between two revisions. Renames
// are not detected: a moved file shows as one delete plus one add.
func (r repo) changed(a, b string) ([]string, error) {
	return r.lines("\x00", "diff", "--name-only", "--no-renames", "-z", a, b)
}

// commits lists the commits in base..head, oldest first.
func (r repo) commits(base, head string) ([]string, error) {
	return r.lines("\n", "rev-list", "--reverse", "--first-parent", base+".."+head)
}

// goFiles lists the .go files directly inside dir at rev.
func (r repo) goFiles(rev, dir string) ([]string, error) {
	args := []string{"ls-tree", "--name-only", "-z", rev}
	if dir != "." {
		args = append(args, dir+"/")
	}
	names, err := r.lines("\x00", args...)
	var res []string
	for _, n := range names {
		if strings.HasSuffix(n, ".go") && path.Dir(n) == dir {
			res = append(res, n)
		}
	}
	return res, err
}

func (r repo) blob(rev, file string) ([]byte, error) {
	return r.git("cat-file", "blob", rev+":"+file)
}
