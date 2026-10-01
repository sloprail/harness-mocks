package hooks

import corehooks "github.com/sloprail/harness-mocks/internal/hooks"

// mergeEntries adds the entries of one more settings file to an event's: a
// handler already there under the same matcher, from another file, runs once
// (docs, Hook handler fields). A plugin's copy is not merged here, so it stays
// separate.
func mergeEntries(have, add []HookEntry) []HookEntry {
	for _, e := range add {
		var same []HandlerSpec // what is already configured under this matcher
		for _, h := range have {
			if h.Matcher == e.Matcher {
				same = append(same, h.Hooks...)
			}
		}
		// sr:provides hooks-all-matching-run/claude
		if fresh := corehooks.Unseen(same, e.Hooks); len(fresh) > 0 {
			have = append(have, HookEntry{Matcher: e.Matcher, Hooks: fresh})
		}
	}
	return have
}
