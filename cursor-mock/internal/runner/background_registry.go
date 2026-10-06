package runner

import (
	"path/filepath"
	"strings"
	"sync"

	"github.com/sloprail/harness-mocks/internal/tasks"
)

// terminalsFolder is where Cursor keeps a project's shells' output, beside its
// transcripts.
func (s *session) terminalsFolder() string {
	project := nonAlnum.ReplaceAllString(strings.TrimPrefix(s.cfg.Dir, "/"), "-")
	return filepath.Join(s.cfg.Home, ".cursor", "projects", project, "terminals")
}

// registries holds each session's background shells (a session is one run, so
// the registry is made on its first background shell, or its first wait).
var registries sync.Map // *session -> *tasks.Registry

func (s *session) registry() *tasks.Registry {
	if s.parent != nil {
		return s.parent.registry()
	}
	r, _ := registries.LoadOrStore(s, tasks.NewRegistry())
	return r.(*tasks.Registry)
}
