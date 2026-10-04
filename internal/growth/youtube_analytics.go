package growth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// Đợt H3 (2026-10-04): YouTube Analytics API cho growth thresholds.
//
// Data API (youtube_api) chỉ cho lifetime totals — Views30d và watch hours
// cần Analytics API (OAuth-only). Source này điền Views30d +
// Extra["watch_hours_30d"] / Extra["subs_gained_30d"] để ngưỡng
// kill/double-down/breakout chạy trên số liệu thật.
//
// MIỄN PHÍ + LOCAL-OK: Analytics API free, chạy qua HTTPS từ binary,
// không tốn RAM M1. Yêu cầu duy nhất: token OAuth có scope
// yt-analytics.readonly (Ninh chạy scripts/get-youtube-token.py một lần).
//
// Fail-closed mọi chỗ: chưa gắn kênh, chưa có token, 401/403 (thiếu scope
// hoặc token hết hạn) → lỗi tiếng Việt rõ ràng, không bịa số.

const youtubeAnalyticsBase = "https://youtubeanalytics.googleapis.com"

// YouTubeAnalyticsSource đọc views/watch-hours 30 ngày từ YouTube Analytics.
type YouTubeAnalyticsSource struct {
	// TokenFor đổi username → OAuth access token.
	// Production: publishers.YouTubeAccessToken. Nil = chưa cấu hình.
	TokenFor func(username string) (string, error)
	// BaseURL override cho test; "" = youtubeAnalyticsBase.
	BaseURL string
	// Client override cho test; nil = http.DefaultClient (timeout 20s).
	Client *http.Client
}

func (y *YouTubeAnalyticsSource) Name() string { return "youtube_analytics" }

func (y *YouTubeAnalyticsSource) base() string {
	if y.BaseURL != "" {
		return y.BaseURL
	}
	return youtubeAnalyticsBase
}

func (y *YouTubeAnalyticsSource) httpClient() *http.Client {
	if y.Client != nil {
		return y.Client
	}
	return &http.Client{Timeout: 20 * time.Second}
}

// NewYouTubeAnalyticsSource builds the source; nil TokenFor = not connected.
func NewYouTubeAnalyticsSource(tokenFor func(username string) (string, error)) *YouTubeAnalyticsSource {
	return &YouTubeAnalyticsSource{TokenFor: tokenFor}
}

func (y *YouTubeAnalyticsSource) Fetch(ctx context.Context, acct SourceAccount) (*Snapshot, error) {
	if acct.YoutubeChannel == "" {
		return nil, fmt.Errorf("%w: chưa gắn kênh YouTube cho tài khoản %s",
			ErrNotConnected, acct.Username)
	}
	if y.TokenFor == nil {
		return nil, fmt.Errorf("%w: YouTube Analytics chưa cấu hình (thiếu token OAuth)",
			ErrNotConnected)
	}
	token, err := y.TokenFor(acct.Username)
	if err != nil || token == "" {
		return nil, fmt.Errorf("%w: không lấy được access token YouTube cho %s: %v",
			ErrNotConnected, acct.Username, err)
	}

	end := time.Now().UTC()
	start := end.AddDate(0, 0, -29) // 30 ngày
	q := url.Values{}
	q.Set("ids", "channel=="+acct.YoutubeChannel)
	q.Set("startDate", start.Format("2006-01-02"))
	q.Set("endDate", end.Format("2006-01-02"))
	q.Set("metrics", "views,estimatedMinutesWatched,subscribersGained")
	endpoint := y.base() + "/v2/reports?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := y.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("youtube analytics: lỗi mạng: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, fmt.Errorf("youtube analytics: token hết hạn hoặc thiếu quyền " +
			"yt-analytics.readonly — chạy lại scripts/get-youtube-token.py để cấp quyền rồi thử lại")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("youtube analytics: HTTP %d: %.200s", resp.StatusCode, body)
	}

	var report struct {
		ColumnHeaders []struct {
			Name string `json:"name"`
		} `json:"columnHeaders"`
		Rows [][]any `json:"rows"`
	}
	if err := json.Unmarshal(body, &report); err != nil {
		return nil, fmt.Errorf("youtube analytics: parse lỗi: %w", err)
	}
	vals := map[string]float64{}
	if len(report.Rows) > 0 {
		for i, h := range report.ColumnHeaders {
			if i < len(report.Rows[0]) {
				vals[h.Name] = numOf(report.Rows[0][i])
			}
		}
	}

	snap := &Snapshot{AccountID: acct.ID, TakenAt: time.Now(), Extra: map[string]any{}}
	if v, ok := vals["views"]; ok {
		vv := v
		snap.Views30d = &vv
	}
	if m, ok := vals["estimatedMinutesWatched"]; ok {
		snap.Extra["watch_hours_30d"] = m / 60
	}
	if s, ok := vals["subscribersGained"]; ok {
		snap.Extra["subs_gained_30d"] = s
	}
	return snap, nil
}

func numOf(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case json.Number:
		f, _ := n.Float64()
		return f
	}
	return 0
}
