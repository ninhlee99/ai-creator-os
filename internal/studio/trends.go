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
	"unicode"
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
	trendsMu         sync.Mutex
	trendsCache      []TrendingSound
	trendsCached     time.Time
	trendsErr        error
	trendsRefreshing bool
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

// TrendsStale reports whether the cached chart is older than the TTL (or
// was never fetched) — the Studio page uses it to self-refresh (Đợt 3).
func TrendsStale() bool {
	trendsMu.Lock()
	defer trendsMu.Unlock()
	return time.Since(trendsCached) >= trendsCacheTTL
}

// RefreshVNTrendingAsync refreshes the chart in the background when stale;
// at most one refresh runs at a time. Errors land in the cache like a
// normal fetch, so a failed refresh never storms the source.
func RefreshVNTrendingAsync() {
	trendsMu.Lock()
	if trendsRefreshing {
		trendsMu.Unlock()
		return
	}
	trendsRefreshing = true
	trendsMu.Unlock()
	go func() {
		defer func() {
			trendsMu.Lock()
			trendsRefreshing = false
			trendsMu.Unlock()
		}()
		_, _ = FetchVNTrending(context.Background())
	}()
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
	kworbRowRe = regexp.MustCompile(`(?s)<tr>\s*<td>(\d+)</td>\s*<td>([^<]*)</td>\s*<td[^>]*>\s*<div>([^<]+)</div>`)
	kworbTagRe = regexp.MustCompile(`<[^>]+>`)
	kworbSpRe  = regexp.MustCompile(`\s+`)
)

func parseKworbVN(html string) []TrendingSound {
	var out []TrendingSound
	for _, m := range kworbRowRe.FindAllStringSubmatch(html, -1) {
		var rank int
		fmt.Sscanf(m[1], "%d", &rank)
		name := kworbSpRe.ReplaceAllString(kworbTagRe.ReplaceAllString(m[3], " "), " ")
		name = sanitizeChartText(name)
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

// sanitizeChartText cleans chart text for display: it strips Unicode
// format/control characters (bidi overrides, zero-width marks) and folds
// stylized letters (mathematical alphanumerics like 𝐅𝐋𝐈:𝐏, fullwidth
// ＦＬＩＰ) back to plain ASCII so titles render in normal fonts instead
// of broken mirrored fallback glyphs. Vietnamese diacritics are untouched.
func sanitizeChartText(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case unicode.Is(unicode.Cf, r) || unicode.Is(unicode.Cc, r):
			// Drop format/control characters entirely.
		case r >= 0xFF01 && r <= 0xFF5E:
			b.WriteRune(r - 0xFEE0) // fullwidth → ASCII
		case r >= 0x1D400 && r <= 0x1D7FF:
			if c, ok := foldMathAlnum(r); ok {
				b.WriteRune(c)
			} else {
				b.WriteRune(r)
			}
		default:
			b.WriteRune(r)
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// foldMathAlnum maps a mathematical alphanumeric rune (U+1D400–U+1D7FF)
// to its plain ASCII letter/digit. The block is laid out as runs of 26
// capitals, 26 smalls, then runs of 10 digits per style.
func foldMathAlnum(r rune) (rune, bool) {
	caps := []rune{0x1D400, 0x1D434, 0x1D468, 0x1D49C, 0x1D4D0, 0x1D504,
		0x1D538, 0x1D56C, 0x1D5A0, 0x1D5D4, 0x1D608, 0x1D63C, 0x1D670}
	smalls := []rune{0x1D41A, 0x1D44E, 0x1D482, 0x1D4B6, 0x1D4EA, 0x1D51E,
		0x1D552, 0x1D586, 0x1D5BA, 0x1D5EE, 0x1D622, 0x1D656, 0x1D68A}
	digits := []rune{0x1D7CE, 0x1D7D8, 0x1D7E2, 0x1D7EC, 0x1D7F6}
	for _, start := range caps {
		if d := r - start; d >= 0 && d < 26 {
			return 'A' + d, true
		}
	}
	for _, start := range smalls {
		if d := r - start; d >= 0 && d < 26 {
			return 'a' + d, true
		}
	}
	for _, start := range digits {
		if d := r - start; d >= 0 && d < 10 {
			return '0' + d, true
		}
	}
	return r, false
}
