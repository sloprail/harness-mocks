package e2e

// notReplaying are the recorded runs whose replay is not green yet, and why.
// The list only shrinks: an entry whose run is gone, or whose run now replays
// green, fails TestGeneratedReplay. "adapter:" is something of the recording
// the cursor adapter cannot reproduce yet; "mock gap:" is what the mock does not
// produce that the recording shows (each gap is a PR of its own). The list as it
// first stood was accepted as it stands, and from then on it may only shrink: no
// entry is added, and an entry may be replaced only when its original gap closes
// and a new one shows.
var notReplaying = map[string]string{}
