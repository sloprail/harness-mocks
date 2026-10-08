package replay

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	core "github.com/sloprail/harness-mocks/internal/replay"
)

// Replay runs the mock on rec's scenario in a hermetic workspace, laid out as a
// capture lays out its own (the project's hooks, the user's home, the temp
// root), and returns the recording's event stream and hook payloads with the
// mock's, normalised. A recording with no stream says nothing to compare, so it
// is not replayable.
func (a Adapter) Replay(mock string, rec core.Recording) (want, got core.Observed, err error) {
	sample := a.sampleDir(rec.Dir)
	recStream, err := readJSONL(filepath.Join(sample, "stream.jsonl"))
	if err != nil {
		return want, got, err
	}
	if len(recStream) == 0 && !isTUI(rec.Setup) { // a TUI has no stream: its hook payloads are what is compared
		return want, got, &Unbuildable{Reason: "the recording has no stream to compare"}
	}
	// No hook log at all is a recording whose hooks never ran (the hook script is what
	// writes it): the mock must run none either. That proves something only when hooks
	// were configured, and the stream compared is not empty (checked above).
	_, statErr := os.Stat(filepath.Join(sample, "payloads.jsonl"))
	noHooksRan := statErr != nil
	if noHooksRan && !hooksConfigured(rec.Setup) {
		return want, got, &Unbuildable{Reason: "the recording has no hook log and configures no hooks: nothing says hooks were left unfired"}
	}
	recHooks, err := readJSONL(filepath.Join(sample, "payloads.jsonl"))
	if err != nil {
		return want, got, err
	}
	work, err := os.MkdirTemp("", "cursor-replay-*")
	if err != nil {
		return want, got, err
	}
	defer os.RemoveAll(work)
	if work, err = filepath.EvalSymlinks(work); err != nil { // canonical, as the capture's paths are
		return want, got, err
	}
	l, err := a.install(work, rec)
	if err != nil {
		return want, got, err
	}
	extra, err := flagWords(rec.Setup["args"], false) // checked at Load
	if err != nil {
		return want, got, err
	}
	stdout, exits, err := a.runSteps(mock, rec, l, extra)
	if err != nil {
		return want, got, err
	}
	hookLog, _ := os.ReadFile(l.hookLog)
	mockStream, err := parseJSONL(stdout)
	if err != nil {
		return want, got, fmt.Errorf("the mock's stream: %w", err)
	}
	mockHooks, err := parseJSONL(string(hookLog))
	if err != nil {
		return want, got, fmt.Errorf("the mock's hook log: %w", err)
	}

	// one canonicalisation per side, the event stream first: it names the ids in a fixed order
	rules := Rules(l.repo, work)
	for _, name := range stepNames(rec.Setup) { // a later step's directory is named in its project folder by the capture's own temp directory
		if cwd := strings.TrimSpace(rec.Setup["then-"+name+"-cwd"]); cwd != "" {
			rules.Scrub = append([]core.Scrub{{Re: regexp.MustCompile(`projects/[A-Za-z0-9-]*-` + regexp.QuoteMeta(cwd) + `/`), With: "projects/<PROJECT>-" + cwd + "/"}}, rules.Scrub...)
		}
	}
	wantC, gotC := core.New(rules), core.New(rules)
	want.Events, got.Events = wantC.Lines(Frames(recStream)), gotC.Lines(Frames(mockStream))
	recExits, _ := exitsOf(rec.Setup["exit"])
	for _, c := range recExits {
		want.Exits = append(want.Exits, "exit "+strconv.Itoa(c))
	}
	for _, c := range exits {
		got.Exits = append(got.Exits, "exit "+strconv.Itoa(c))
	}
	// what the transcript records of a Write is compared too: its absolute path and its
	// "contents" (recorded: runs/file-tools), not the frame's relative path and streamContent
	recWrites, err := transcriptWrites(filepath.Join(sample, "transcript"))
	if err != nil {
		return want, got, err
	}
	mockWrites, err := transcriptWrites(filepath.Join(work, "home", ".cursor", "projects"))
	if err != nil {
		return want, got, err
	}
	want.Events = append(want.Events, sortedLines(wantC.Lines(recWrites))...)
	got.Events = append(got.Events, sortedLines(gotC.Lines(mockWrites))...)
	recHooks, mockHooks = inOrder(recHooks), inOrder(mockHooks)
	unsettled(recHooks)
	unsettled(mockHooks)
	want.NoStream, got.NoStream = isTUI(rec.Setup), isTUI(rec.Setup)
	want.Hooks, got.Hooks = concurrent(recHooks, wantC.Lines(recHooks)), concurrent(mockHooks, gotC.Lines(mockHooks))
	if len(want.Hooks) == 0 && len(got.Hooks) == 0 && !noHooksRan {
		return want, got, &Unbuildable{Reason: "the recording's hook log holds no line the mock models: nothing to compare"}
	}
	return want, got, nil
}

// sampleDir is the sample this adapter replays of the run in dir.
func (a Adapter) sampleDir(dir string) string {
	if a.Sample != "" {
		return filepath.Join(dir, "samples", a.Sample)
	}
	return sampleDir(dir)
}

// the process environment a capture gives cursor-agent: a home and a temp root
// of its own and the hook log, with only the tools the hooks use from ours.
func (a Adapter) baseEnv(home, tmp, hookLog string) []string {
	env := []string{"HOME=" + home, "TMPDIR=" + tmp, "HOOK_LOG=" + hookLog,
		"GIT_AUTHOR_NAME=replay", "GIT_AUTHOR_EMAIL=replay@sloprail.invalid", "GIT_COMMITTER_NAME=replay", "GIT_COMMITTER_EMAIL=replay@sloprail.invalid"}
	for _, kv := range a.Environ {
		if k, _, _ := strings.Cut(kv, "="); k == "PATH" || k == "USER" || k == "LANG" || k == "TERM" {
			env = append(env, kv)
		}
	}
	return env
}
