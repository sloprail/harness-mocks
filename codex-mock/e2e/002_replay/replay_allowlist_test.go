package e2e

// notReplaying are the recorded runs whose replay is not green yet, and why.
// The list only shrinks: an entry whose run is gone, or whose run now replays
// green, fails TestGeneratedReplay (except a "flaky:" entry, which is green in
// some runs and so is only checked for its run being there). "adapter:" is
// something of the recording the codex adapter cannot reproduce yet;
// "untriaged:" is a replay that differs and has not been looked at (the mock,
// the adapter or the recording may be wrong).
var notReplaying = map[string]string{
	"noninteractive-run-output-schema": "mock gap: the recording ran with --output-schema, which the mock refuses by design (adr/fail-fast-unimplemented); replaying it needs the decision that a refused-flag recording replays as a check that the mock refuses it",
}
