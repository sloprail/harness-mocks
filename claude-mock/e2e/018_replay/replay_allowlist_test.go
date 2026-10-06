package e2e

// notReplaying are the recorded runs whose replay is not green yet, and why.
// The list only shrinks: an entry whose run is gone, or whose run now replays
// green, fails TestGeneratedReplay. "adapter:" is something of the recording
// the claude adapter cannot reproduce yet; "mock gap:" is what the mock does not produce that
// the recording shows (each gap is a PR of its own).
var notReplaying = map[string]string{
	"bgagent":                  "adapter: the model said two things before one call: the adapter keeps one",
	"bgagent-concurrent-limit": "adapter: the setup has env, which the adapter does not install",
	"bgagent-definition":       "adapter: the model said two things before one call: the adapter keeps one",
	"bgagent-nested-launcher":  "adapter: the model said two things before one call: the adapter keeps one",
	"fgsub-maxturns":           "adapter: the model called SendMessage: the adapter maps Bash and Agent",
	"nested-fork-limit":        "adapter: the setup has env, which the adapter does not install",
}
