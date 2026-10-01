package session

import (
	"testing"
	"time"
)

func TestStats(t *testing.T) {
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	got := Stats(now.Add(-90*time.Minute), now, time.Hour, 182340, 6.25)
	if got.SecondsSinceLastResponse != 5400 || got.ContextTokens != 182340 || !got.CacheLikelyExpired || got.EstimatedCacheWriteUSD != 1.1396 {
		t.Fatalf("stats = %+v", got)
	}
	fresh := Stats(now.Add(-1500*time.Millisecond), now, time.Hour, 23551, 2)
	if fresh.SecondsSinceLastResponse != 2 || fresh.CacheLikelyExpired || fresh.EstimatedCacheWriteUSD != 0.0471 {
		t.Fatalf("fresh = %+v", fresh)
	}
	none := Stats(time.Time{}, now, time.Hour, 0, 2)
	if none.SecondsSinceLastResponse != 0 || none.CacheLikelyExpired || none.EstimatedCacheWriteUSD != 0 {
		t.Fatalf("no last response = %+v", none)
	}
	if future := Stats(now.Add(time.Minute), now, time.Hour, 0, 0); future.SecondsSinceLastResponse != 0 {
		t.Fatalf("a last response after now = %+v", future)
	}
}
