package e2e

// notReplaying are the recorded runs whose replay is not green yet, and why.
// The list only shrinks: an entry whose run is gone, or whose run now replays
// green, fails TestGeneratedReplay (except a "flaky:" entry, which is green in
// some runs and so is only checked for its run being there). "adapter:" is
// something of the recording the codex adapter cannot reproduce yet;
// "untriaged:" is a replay that differs and has not been looked at (the mock,
// the adapter or the recording may be wrong).
var notReplaying = map[string]string{}
