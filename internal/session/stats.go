package session

import "time"

// ResumeStats is what a resumed session's start hook is told of the resume: how
// long it has been since the last response, how many tokens the first request
// re-sends, whether the prompt cache is likely gone, and what writing the context
// into it again is estimated to cost.
type ResumeStats struct {
	SecondsSinceLastResponse int
	ContextTokens            int
	CacheLikelyExpired       bool
	EstimatedCacheWriteUSD   float64
}

// Stats is the stats of resuming at now a session whose last response was at
// lastResponse (the zero time when it has none): the cache is likely gone when
// the last response is older than cacheLifetime, and a rewrite of contextTokens
// costs usdPerMillionTokens per million of them, rounded to a hundredth of a cent.
func Stats(lastResponse, now time.Time, cacheLifetime time.Duration, contextTokens int, usdPerMillionTokens float64) ResumeStats {
	var since time.Duration
	if !lastResponse.IsZero() {
		since = max(now.Sub(lastResponse), 0)
	}
	usd := float64(contextTokens) * usdPerMillionTokens / 1e6
	return ResumeStats{
		SecondsSinceLastResponse: int(since.Round(time.Second) / time.Second),
		ContextTokens:            contextTokens,
		CacheLikelyExpired:       !lastResponse.IsZero() && since > cacheLifetime,
		EstimatedCacheWriteUSD:   float64(int(usd*10000+0.5)) / 10000,
	}
}
