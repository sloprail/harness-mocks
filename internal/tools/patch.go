package tools

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FileChange is one file a patch touched; Kind is "add", "update" or "delete".
type FileChange struct{ Path, Kind string }

// Patch errors, each wrapped with the file and lines it concerns.
var (
	// ErrPatchMalformed: the text is not an envelope of file sections.
	ErrPatchMalformed = errors.New("invalid patch")
	// ErrPatchNoFile: a file to update is not there.
	ErrPatchNoFile = errors.New("file to update not found")
	// ErrPatchNoMatch: the lines a hunk names are not in the file.
	ErrPatchNoMatch = errors.New("lines to replace not found")
)

type patchWrite struct {
	path, content string
	remove        bool
}

// ApplyPatch carries out a patch on the files under dir (a relative path is
// dir's): "*** Add File:" creates a file, or replaces one whole, with its "+"
// lines; "*** Update File:" replaces, for each "@@" hunk, the lines the hunk's
// context and "-" lines name with its context and "+" lines; "*** Delete File:"
// removes a file. Nothing is written unless every section applies; the error
// of one that does not wraps a Patch error and names the file.
//
// sr:capability file-tools
func ApplyPatch(patch, dir string) ([]FileChange, error) {
	lines := strings.Split(strings.TrimRight(patch, "\n"), "\n")
	if len(lines) < 2 || lines[0] != "*** Begin Patch" || lines[len(lines)-1] != "*** End Patch" {
		return nil, ErrPatchMalformed
	}
	body := lines[1 : len(lines)-1]
	var writes []patchWrite
	var changes []FileChange
	abs := func(head, prefix string) string {
		p := strings.TrimPrefix(head, prefix)
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
			p := abs(head, "*** Add File: ")
			var text strings.Builder
			for _, l := range sect {
				text.WriteString(strings.TrimPrefix(l, "+") + "\n")
			}
			writes = append(writes, patchWrite{path: p, content: text.String()})
			changes = append(changes, FileChange{p, "add"})
		case strings.HasPrefix(head, "*** Delete File: "):
			p := abs(head, "*** Delete File: ")
			writes = append(writes, patchWrite{path: p, remove: true})
			changes = append(changes, FileChange{p, "delete"})
		case strings.HasPrefix(head, "*** Update File: "):
			p := abs(head, "*** Update File: ")
			content, err := ReadFile(p)
			if err != nil {
				return nil, fmt.Errorf("%w: %s", ErrPatchNoFile, p)
			}
			if content, err = applyHunks(content, sect, p); err != nil {
				return nil, err
			}
			writes = append(writes, patchWrite{path: p, content: content})
			changes = append(changes, FileChange{p, "update"})
		default:
			return nil, ErrPatchMalformed
		}
	}
	for _, w := range writes {
		var err error
		if w.remove {
			err = os.Remove(w.path)
		} else {
			_, _, err = WriteFile(w.path, w.content)
		}
		if err != nil {
			return nil, err
		}
	}
	return changes, nil
}

// applyHunks applies the "@@" hunks of an update section, each to the first
// place its old lines (context and "-") appear.
func applyHunks(content string, sect []string, path string) (string, error) {
	var oldLines, newLines []string
	flush := func() error {
		if len(oldLines)+len(newLines) == 0 {
			return nil
		}
		old, repl := strings.Join(oldLines, "\n")+"\n", strings.Join(newLines, "\n")+"\n"
		if !strings.Contains(content, old) {
			return fmt.Errorf("%w: %s:\n%s", ErrPatchNoMatch, path, strings.Join(oldLines, "\n"))
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
