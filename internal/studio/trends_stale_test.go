package studio

import (
	"testing"
	"time"
)

// Đợt 3 (A6): the Studio page self-refreshes a stale chart. Only the
// staleness logic is pinned here — RefreshVNTrendingAsync itself hits
// the network and stays out of unit tests.
func TestTrendsStale(t *testing.T) {
	trendsMu.Lock()
	origCache, origCached, origErr := trendsCache, trendsCached, trendsErr
	trendsCache, trendsCached, trendsErr = nil, time.Time{}, nil
	trendsMu.Unlock()
	defer func() {
		trendsMu.Lock()
		trendsCache, trendsCached, trendsErr = origCache, origCached, origErr
		trendsMu.Unlock()
	}()

	if !TrendsStale() {
		t.Fatal("never-fetched chart must be stale")
	}
	trendsMu.Lock()
	trendsCached = time.Now()
	trendsMu.Unlock()
	if TrendsStale() {
		t.Fatal("freshly cached chart must not be stale")
	}
	trendsMu.Lock()
	trendsCached = time.Now().Add(-trendsCacheTTL - time.Minute)
	trendsMu.Unlock()
	if !TrendsStale() {
		t.Fatal("chart older than the TTL must be stale")
	}
}
