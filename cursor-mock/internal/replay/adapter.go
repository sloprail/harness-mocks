// Package replay is the cursor mock's replay adapter: every cursor-specific
// decision of replaying a recording is here (reading the transcripts into turns,
// writing the mock's scenario script, running the mock, which fields are not
// behaviour). The shared core (internal/replay) speaks only the unified format
// and compares what the adapter normalises.
package replay

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	core "github.com/sloprail/harness-mocks/internal/replay"
)

// Adapter is the cursor harness's side of a replay (core.Adapter): it reads a
// cursor recording into the unified form (Load), turns that into the scenario
// the cursor mock takes (Denormalize), runs the mock (Replay), and normalises
// both outputs by cursor's own rules (Rules).
type Adapter struct {
	// Environ is the environment the entrypoint was started with; only what the hooks' own tools need is passed on.
	Environ []string
	// Sample is the sample of the run to replay: its folder name under samples/; empty is the newest.
	Sample string
}

// Run replays every sample of the recording in runDir: the mock binary is run on
// the scenario generated from each, and what differs from the recording is
// returned; empty is a green replay. A recording the adapter cannot build is an
// *Unbuildable.
func Run(mock, runDir string, environ []string) (string, error) {
	samples, _ := filepath.Glob(filepath.Join(runDir, "samples", "*"))
	if len(samples) == 0 {
		return "", &Unbuildable{Reason: "no sample was recorded"}
	}
	sort.Strings(samples)
	var out strings.Builder
	for _, s := range samples {
		diff, err := core.Run(Adapter{Environ: environ, Sample: filepath.Base(s)}, mock, runDir)
		if err != nil {
			return "", err
		}
		if diff != "" {
			fmt.Fprintf(&out, "sample %s:\n%s", filepath.Base(s), diff)
		}
	}
	return out.String(), nil
}

// Script is the generated scenario for debugging: the main script and each
// sub-agent's, for the newest sample.
func Script(runDir string) (string, error) { return core.Script(Adapter{}, runDir) }

// Script is the scenario Denormalize makes of rec, as text.
func (Adapter) Script(rec core.Recording) (string, error) {
	s := Denormalize(rec, "<scripts>", nil)
	out := "# main\n" + s.Script
	scripts := map[string]string{}
	for name, body := range s.Scripts {
		scripts[name] = body
	}
	lines := map[string]int{"repo": s.Lines}
	for i, st := range rec.Then {
		project := stepProject(rec.Setup, stepNames(rec.Setup)[i])
		sc := DenormalizeStep(st, i+1, "<scripts>", nil, lines[project])
		lines[project] += sc.Lines
		scripts[fmt.Sprintf("main%d.sh", i+1)] = sc.Script
		for name, body := range sc.Scripts {
			scripts[name] = body
		}
	}
	names := make([]string, 0, len(scripts))
	for name := range scripts {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		out += "\n# " + name + "\n" + scripts[name]
	}
	return out, nil
}
