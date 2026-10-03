// Package toolexec runs the file tool of Codex: apply_patch, the one way the
// agent writes files (it reads them through the shell).
package toolexec

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sloprail/harness-mocks/internal/tools"
)

// Change is one file a patch touched; Kind is "add", "update" or "delete", the
// words the event stream's file_change item uses.
type Change struct{ Path, Kind string }

// ErrMalformed is a patch that is not an envelope of file sections.
var ErrMalformed = errors.New("apply_patch verification failed: invalid patch")

type write struct {
	path, content string
	remove        bool
}

// Apply carries out a patch on the files under dir (relative paths are dir's):
// "*** Add File:" creates a file, or replaces one whole, with its "+" lines;
// "*** Update File:" replaces, for each "@@" hunk, the lines the hunk's context
// and "-" lines name with its context and "+" lines; "*** Delete File:"
// removes a file. Nothing is written unless every section applies. On success
// it returns the files it changed, and the text Codex's hooks see as the tool's
// response (recorded: runs/file-tools); a failure is the error the agent is
// told, in Codex's words (recorded: runs/file-tools-failure).
//
// sr:provides file-tools/codex
func Apply(patch, dir string) (changes []Change, response string, err error) {
	lines := strings.Split(strings.TrimRight(patch, "\n"), "\n")
	if len(lines) < 2 || lines[0] != "*** Begin Patch" || lines[len(lines)-1] != "*** End Patch" {
		return nil, "", ErrMalformed
	}
	body := lines[1 : len(lines)-1]
	var writes []write
	abs := func(p string) string {
		if filepath.IsAbs(p) {
			return p
		}
		return filepath.Join(dir, p)
	}
	for i := 0; i < len(body); {
		head := body[i]
		i++
		j := i
		for j < len(body) && !strings.HasPrefix(body[j], "*** ") {
			j++
		}
		sect := body[i:j]
		i = j
		switch {
		case strings.HasPrefix(head, "*** Add File: "):
			p := abs(strings.TrimPrefix(head, "*** Add File: "))
			var text strings.Builder
			for _, l := range sect {
				text.WriteString(strings.TrimPrefix(l, "+") + "\n")
			}
			writes = append(writes, write{path: p, content: text.String()})
			changes = append(changes, Change{p, "add"})
		case strings.HasPrefix(head, "*** Delete File: "):
			p := abs(strings.TrimPrefix(head, "*** Delete File: "))
			writes = append(writes, write{path: p, remove: true})
			changes = append(changes, Change{p, "delete"})
		case strings.HasPrefix(head, "*** Update File: "):
			p := abs(strings.TrimPrefix(head, "*** Update File: "))
			content, rerr := tools.ReadFile(p)
			if rerr != nil {
				return nil, "", fmt.Errorf("apply_patch verification failed: Failed to read file to update %s: No such file or directory (os error 2)", p)
			}
			content, err = hunks(content, sect, p)
			if err != nil {
				return nil, "", err
			}
			writes = append(writes, write{path: p, content: content})
			changes = append(changes, Change{p, "update"})
		default:
			return nil, "", ErrMalformed
		}
	}
	response = "Exit code: 0\nWall time: 0 seconds\nOutput:\nSuccess. Updated the following files:\n"
	for k, w := range writes {
		if w.remove {
			err = os.Remove(w.path)
		} else {
			_, _, err = tools.WriteFile(w.path, w.content)
		}
		if err != nil {
			return nil, "", err
		}
		response += map[string]string{"add": "A", "update": "M", "delete": "D"}[changes[k].Kind] + " " + w.path + "\n"
	}
	return changes, response, nil
}

// hunks applies the "@@" hunks of an update section, each to the first place
// its old lines (context and "-") appear.
func hunks(content string, sect []string, path string) (string, error) {
	var oldLines, newLines []string
	flush := func() error {
		if len(oldLines)+len(newLines) == 0 {
			return nil
		}
		old, repl := strings.Join(oldLines, "\n")+"\n", strings.Join(newLines, "\n")+"\n"
		if !strings.Contains(content, old) {
			return fmt.Errorf("apply_patch verification failed: Failed to find expected lines in %s:\n%s", path, strings.Join(oldLines, "\n"))
		}
		content = strings.Replace(content, old, repl, 1)
		oldLines, newLines = nil, nil
		return nil
	}
	for _, l := range sect {
		switch {
		case strings.HasPrefix(l, "@@"):
			if err := flush(); err != nil {
				return "", err
			}
		case strings.HasPrefix(l, "-"):
			oldLines = append(oldLines, l[1:])
		case strings.HasPrefix(l, "+"):
			newLines = append(newLines, l[1:])
		default:
			oldLines = append(oldLines, strings.TrimPrefix(l, " "))
			newLines = append(newLines, strings.TrimPrefix(l, " "))
		}
	}
	if err := flush(); err != nil {
		return "", err
	}
	return content, nil
}
