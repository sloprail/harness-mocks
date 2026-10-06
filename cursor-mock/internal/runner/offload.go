package runner

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/sloprail/harness-mocks/cursor-mock/internal/toolexec"
	coresession "github.com/sloprail/harness-mocks/internal/session"
)

// agentToolsFolder is where Cursor keeps the output of a command too big for its
// frame, beside the project's transcripts and terminals.
func (s *session) agentToolsFolder() string {
	return filepath.Join(filepath.Dir(s.terminalsFolder()), "agent-tools")
}

// offload keeps a command's output that is too big for its frame in a file of the
// harness's own: the frame then has no stdout or stderr, says where the output is and
// how big, and the whole output stays in interleavedOutput (recorded:
// runs/compaction-transcript-continuity, where the agent then reads the file).
func (s *session) offload(res *toolexec.Result) {
	if len(res.Output) <= toolexec.BigBytes {
		return
	}
	for _, kind := range []string{"success", "failure"} {
		body, ok := res.Frame[kind].(map[string]any)
		if !ok {
			continue
		}
		dir := s.agentToolsFolder()
		path := filepath.Join(dir, coresession.NewID()+".txt")
		if os.MkdirAll(dir, 0o755) != nil || os.WriteFile(path, []byte(res.Output), 0o644) != nil {
			return
		}
		offloads.add(s, path)
		body["stdout"], body["stderr"] = "", ""
		body["outputLocation"] = map[string]any{
			"filePath": path, "sizeBytes": strconv.Itoa(len(res.Output)), "lineCount": strconv.Itoa(strings.Count(res.Output, "\n")),
		}
	}
}

// offloaded are a session's kept outputs, in the order they were made, and the names a
// script has read them by.
type offloaded struct {
	mu      sync.Mutex
	files   []string
	claimed map[string]string // the path a script names -> the file it stands for
	next    int
}

type offloadSets struct{ m sync.Map } // *session -> *offloaded

var offloads offloadSets

func (o *offloadSets) of(s *session) *offloaded {
	v, _ := o.m.LoadOrStore(s, &offloaded{claimed: map[string]string{}})
	return v.(*offloaded)
}

func (o *offloadSets) add(s *session, path string) {
	f := o.of(s)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.files = append(f.files, path)
}

// resolveOffload is the file a script's Read of a kept output stands for. A real agent
// reads the path the frame gave; a script is written before the run, with the path its
// recording's run was given, so the first read of an unknown path in the agent-tools
// folder takes the first output no read has taken, and the path then stands for it.
func (s *session) resolveOffload(path string) string {
	if filepath.Dir(path) != s.agentToolsFolder() {
		return path
	}
	if _, err := os.Stat(path); err == nil {
		return path
	}
	f := offloads.of(s)
	f.mu.Lock()
	defer f.mu.Unlock()
	if file, ok := f.claimed[path]; ok {
		return file
	}
	if f.next >= len(f.files) {
		return path
	}
	f.claimed[path] = f.files[f.next]
	f.next++
	return f.claimed[path]
}
