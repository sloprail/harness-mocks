package hooks

// AfterRefusedTool is the hook a call that a before-tool hook refused fires:
// what differs between harnesses. Some fire neither hook, since the tool did
// not run (AfterToolHook); others fire the failure hook, so a refusal is
// reported as a failed call. failureFires says which the harness does.
func AfterRefusedTool(failureFires bool) AfterTool {
	if failureFires {
		return AfterFailure
	}
	return AfterNone
}
