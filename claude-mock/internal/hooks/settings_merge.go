package hooks

// mergeEntries adds the entries of one more settings file to an event's: a
// handler already there under the same matcher, from another file, runs once
// (docs, Hook handler fields). A plugin's copy is not merged here, so it stays
// separate.
func mergeEntries(have, add []HookEntry) []HookEntry {
	for _, e := range add {
		var fresh []HandlerSpec
		for _, h := range e.Hooks {
			if !hasHandler(have, e.Matcher, h) {
				fresh = append(fresh, h)
			}
		}
		if len(fresh) > 0 {
			have = append(have, HookEntry{Matcher: e.Matcher, Hooks: fresh})
		}
	}
	return have
}

func hasHandler(entries []HookEntry, matcher string, h HandlerSpec) bool {
	for _, e := range entries {
		if e.Matcher != matcher {
			continue
		}
		for _, o := range e.Hooks {
			if o == h {
				return true
			}
		}
	}
	return false
}
