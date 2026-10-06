package replay

import "time"

// stampOf is the time a rollout record was made, zero when it has none: what the
// order of one agent's steps against another's is read from.
func stampOf(rec map[string]any) time.Time {
	s, _ := rec["timestamp"].(string)
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}
	}
	return t
}
