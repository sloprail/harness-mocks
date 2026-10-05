package replay

import (
	"path/filepath"
	"sort"

	core "github.com/sloprail/harness-mocks/internal/replay"
)

// sampleDirs are the samples of the run in dir, oldest first. A replay is green
// only when the mock's output matches every one of them.
func sampleDirs(dir string) []string {
	samples, _ := filepath.Glob(filepath.Join(dir, "samples", "*"))
	sort.Strings(samples)
	return samples
}

// observe is a recording's event stream and hook payloads with the mock's, each
// side canonicalised under rules, the event stream first: it names the ids in a
// fixed order.
func observe(rules core.Rules, recStream, recHooks, mockStream, mockHooks []map[string]any) (want, got core.Observed) {
	wantC, gotC := core.New(rules), core.New(rules)
	want.Events, got.Events = wantC.Lines(Frames(recStream)), gotC.Lines(Frames(mockStream))
	want.Hooks, got.Hooks = wantC.Lines(HookPayloads(recHooks)), gotC.Lines(HookPayloads(mockHooks))
	sortConcurrent(want.Hooks, recHooks)
	sortConcurrent(got.Hooks, mockHooks)
	return want, got
}

// Replay runs the mock once on the scenario of each sample of the recording
// (the model's turns differ from sample to sample), and returns that sample's
// event stream and hook payloads with the mock's, normalised, one section per
// sample under a header: a replay is green only when every sample is.
func (a Adapter) Replay(mock string, rec core.Recording) (want, got core.Observed, err error) {
	for _, sample := range sampleDirs(rec.Dir) {
		one, err := a.LoadSample(rec.Dir, sample)
		if err != nil {
			return want, got, err
		}
		mockStream, mockHooks, repo, work, err := a.runMock(mock, one)
		if err != nil {
			return want, got, err
		}
		recStream, err := readJSONL(filepath.Join(sample, "stream.jsonl"))
		if err != nil {
			return want, got, err
		}
		recHooks, err := readHooks(filepath.Join(sample, "payloads.jsonl"))
		if err != nil {
			return want, got, err
		}
		w, g := observe(Rules(repo, work, RunIDs(recStream, mockStream, recHooks, mockHooks)), recStream, recHooks, mockStream, mockHooks)
		header := "sample " + filepath.Base(sample)
		want.Events, got.Events = append(append(want.Events, header), w.Events...), append(append(got.Events, header), g.Events...)
		want.Hooks, got.Hooks = append(append(want.Hooks, header), w.Hooks...), append(append(got.Hooks, header), g.Hooks...)
	}
	return want, got, nil
}
