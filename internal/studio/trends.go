package studio

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

// TrendingSound is one row of the Vietnam TikTok trending-sounds chart.
type TrendingSound struct {
	Rank     int    `json:"rank"`
	Movement string `json:"movement"` // "=", "+", "-" with steps, e.g. "+19"
	Artist   string `json:"artist"`
	Title    string `json:"title"`
}

// TrendsSource names where the chart came from (shown honestly in the UI).
const TrendsSource = "Kworb — TikTok Trending Songs Vietnam (kworb.net/charts/tiktok/vn.html)"

const kworbVNURL = "https://kworb.net/charts/tiktok/vn.html"

// trendsCacheTTL: the chart moves daily; 6h keeps the dashboard fresh
// without hammering the source.
const trendsCacheTTL = 6 * time.Hour

var (
	trendsMu     sync.Mutex
	trendsCache  []TrendingSound
	trendsCached time.Time
	trendsErr    error
)

// FetchVNTrending returns the current TikTok-Vietnam trending sounds,
// cached for trendsCacheTTL. It reports the source honestly: this is a
// third-party chart, not TikTok's official Creative Center data.
func FetchVNTrending(ctx context.Context) ([]TrendingSound, error) {
	trendsMu.Lock()
	if time.Since(trendsCached) < trendsCacheTTL && (len(trendsCache) > 0 || trendsErr != nil) {
		defer trendsMu.Unlock()
		return append([]TrendingSound(nil), trendsCache...), trendsErr
	}
	trendsMu.Unlock()

	sounds, err := fetchKworbVN(ctx)

	trendsMu.Lock()
	trendsCache = sounds
	trendsCached = time.Now()
	trendsErr = err
	trendsMu.Unlock()

	if err != nil {
		return nil, err
	}
	return append([]TrendingSound(nil), sounds...), nil
}

// RefreshVNTrending forces a fresh fetch (dashboard "làm mới" button).
func RefreshVNTrending(ctx context.Context) ([]TrendingSound, error) {
	trendsMu.Lock()
	trendsCached = time.Time{}
	trendsMu.Unlock()
	return FetchVNTrending(ctx)
}

func fetchKworbVN(ctx context.Context) ([]TrendingSound, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, kworbVNURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36")
	client := &http.Client{Timeout: 25 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("kworb fetch: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("kworb HTTP %d", resp.StatusCode)
	}
	html, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	sounds := parseKworbVN(string(html))
	if len(sounds) == 0 {
		return nil, fmt.Errorf("kworb: no chart rows parsed (page layout may have changed)")
	}
	return sounds, nil
}

// Rows look like:
// <tr><td>1</td><td>=</td><td class="mp text"><div>Artist - Title</div></td>
var (
	kworbRowRe  = regexp.MustCompile(`(?s)<tr>\s*<td>(\d+)</td>\s*<td>([^<]*)</td>\s*<td[^>]*>\s*<div>([^<]+)</div>`)
	kworbTagRe  = regexp.MustCompile(`<[^>]+>`)
	kworbSpRe   = regexp.MustCompile(`\s+`)
)

func parseKworbVN(html string) []TrendingSound {
	var out []TrendingSound
	for _, m := range kworbRowRe.FindAllStringSubmatch(html, -1) {
		var rank int
		fmt.Sscanf(m[1], "%d", &rank)
		name := kworbSpRe.ReplaceAllString(kworbTagRe.ReplaceAllString(m[3], " "), " ")
		name = strings.TrimSpace(name)
		artist, title := splitArtistTitle(name)
		out = append(out, TrendingSound{
			Rank:     rank,
			Movement: strings.TrimSpace(m[2]),
			Artist:   artist,
			Title:    title,
		})
		if len(out) >= 50 {
			break
		}
	}
	return out
}

// splitArtistTitle splits "Artist - Title" on the first " - ".
func splitArtistTitle(name string) (artist, title string) {
	if i := strings.Index(name, " - "); i >= 0 {
		return strings.TrimSpace(name[:i]), strings.TrimSpace(name[i+3:])
	}
	return "", name
}
