package hooks

// ResumeFields are the SessionStart fields only a resumed session carries (recorded:
// snapshots/runs/forkresume and compact; hooks#sessionstart-input).
type ResumeFields struct {
	SecondsSinceLastResponse int     `json:"seconds_since_last_response"`
	ContextTokens            int     `json:"context_tokens"`
	PromptCacheLikelyExpired bool    `json:"prompt_cache_likely_expired"`
	EstimatedCacheWriteUSD   float64 `json:"estimated_cache_write_usd"`
}
