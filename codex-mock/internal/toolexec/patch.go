// Package toolexec is Codex's side of its file tool, apply_patch (the one way
// the agent writes files; it reads them through the shell): the patch is
// applied by the core, and this puts the result in Codex's words.
package toolexec

import (
	"errors"
	"fmt"
	"strings"

	"github.com/sloprail/harness-mocks/internal/tools"
)

// Change is one file a patch touched; Kind is "add", "update" or "delete", the
// words the event stream's file_change item uses.
type Change struct{ Path, Kind string }

// Apply applies a patch to the files under dir. On success it returns the
// files changed and the text Codex's hooks see as the tool's response
// (recorded: runs/file-tools); a failure is the error the agent is told, in
// Codex's words (recorded: runs/file-tools-failure).
//
// sr:provides file-tools/codex
func Apply(patch, dir string) ([]Change, string, error) {
	done, err := tools.ApplyPatch(patch, dir)
	if err != nil {
		return nil, "", wording(err)
	}
	response := "Exit code: 0\nWall time: 0 seconds\nOutput:\nSuccess. Updated the following files:\n"
	var changes []Change
	for _, c := range done {
		changes = append(changes, Change(c))
		response += map[string]string{"add": "A", "update": "M", "delete": "D"}[c.Kind] + " " + c.Path + "\n"
	}
	return changes, response, nil
}

// wording is a patch error in the words of Codex's verification failure.
func wording(err error) error {
	const prefix = "apply_patch verification failed: "
	switch {
	case errors.Is(err, tools.ErrPatchNoFile):
		path := strings.TrimPrefix(err.Error(), tools.ErrPatchNoFile.Error()+": ")
		return fmt.Errorf("%sFailed to read file to update %s: No such file or directory (os error 2)", prefix, path)
	case errors.Is(err, tools.ErrPatchNoMatch):
		return fmt.Errorf("%sFailed to find expected lines in %s",
			prefix, strings.TrimPrefix(err.Error(), tools.ErrPatchNoMatch.Error()+": "))
	}
	return fmt.Errorf("%s%w", prefix, err)
}
