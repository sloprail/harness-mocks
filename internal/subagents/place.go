// Package subagents is the harness-neutral core of a nested agent run: where it
// is placed, where its transcript lives, its hooks, its isolation and what it
// hands back.
package subagents

import (
	"path/filepath"
	"strings"
)

// Layout is where a harness keeps sub-agent transcripts: in one directory
// beside the session's own transcript.
type Layout struct {
	// SessionExt is the session transcript's extension, which the directory
	// name drops: <session transcript without ext>/<Dir>/<Prefix><id><Ext>.
	SessionExt, Dir, Prefix, Ext string
	// SidecarExt replaces Ext on a sub-agent's transcript path to name the
	// metadata file that sits beside it.
	SidecarExt string
	// For a harness with no sub-agent directory (Dir empty), the transcript is
	// kept with the session's: next to its file, or with OwnDir in a directory
	// named by the sub-agent's id next to the session's directory.
	OwnDir bool
}

// Path is the transcript of sub-agent id for a session whose transcript is
// sessionFile: a file of its own in the session's sub-agent directory, never
// the session's. A nested sub-agent's is in the same directory as its parent's,
// since every sub-agent of a session is recorded there.
//
// sr:capability subagent-transcripts
func (l Layout) Path(sessionFile, id string) string {
	if l.Dir == "" {
		dir := filepath.Dir(sessionFile)
		if l.OwnDir {
			dir = filepath.Join(filepath.Dir(dir), id)
		}
		return filepath.Join(dir, l.Prefix+id+l.Ext)
	}
	return filepath.Join(strings.TrimSuffix(sessionFile, l.SessionExt), l.Dir, l.Prefix+id+l.Ext)
}

// Sidecar is the metadata file beside the sub-agent transcript at path: the
// same name with the sidecar extension.
func (l Layout) Sidecar(path string) string {
	return strings.TrimSuffix(path, l.Ext) + l.SidecarExt
}

// Parent is the agent that dispatches a sub-agent: the main thread (ID "",
// depth 0) or a sub-agent.
type Parent struct {
	ID    string
	Depth int
}

// Placement is where a sub-agent sits in the tree of a session's agents.
type Placement struct {
	// Depth is how deep in sub-agents it is: 1 for a sub-agent of the main
	// thread, 2 for one that sub-agent dispatched.
	Depth int
	// ParentID is the dispatching sub-agent's id, "" for the main thread.
	ParentID string
}

// Place is the placement of a sub-agent dispatched by parent: one deeper, naming
// the parent. A sub-agent can itself dispatch sub-agents.
//
// sr:capability nested-subagents
func Place(parent Parent) Placement {
	return Placement{Depth: parent.Depth + 1, ParentID: parent.ID}
}

// DefaultSpawnLimit is how many layers of sub-agents a session can nest below
// its main thread when the harness is not told otherwise.
const DefaultSpawnLimit = 3

// CanDispatch reports whether parent may dispatch a sub-agent when a session
// nests at most limit layers deep (a limit of 0 is the default, a negative or
// 1 turns nesting off for sub-agents): the main thread always may, and a
// sub-agent only while it is above the limit.
func (p Parent) CanDispatch(limit int) bool {
	if limit == 0 {
		limit = DefaultSpawnLimit
	}
	return p.Depth < limit
}
