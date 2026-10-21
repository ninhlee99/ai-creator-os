package growth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
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
	if !strings.HasPrefix(strings.TrimSpace(acct.YoutubeChannel), "UC") {
		return nil, fmt.Errorf("%w: YouTube Analytics cần Channel ID bắt đầu bằng UC; @handle hiện chỉ dùng được với YouTube Data API",
			ErrNotConnected)
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
	// Tiến độ YPP (best-effort — lỗi thì giữ snapshot 30 ngày ở trên).
	if m, err := y.report(ctx, token, acct.YoutubeChannel, 365, "estimatedMinutesWatched", ""); err == nil {
		if v, ok := m["estimatedMinutesWatched"]; ok {
			snap.Extra["watch_hours_12m"] = v / 60
		}
	}
	if m, err := y.report(ctx, token, acct.YoutubeChannel, 90, "views", "creatorContentType"); err == nil {
		snap.Extra["shorts_views_90d"] = m["SHORTS"]
	}
	return snap, nil
}

// FetchVideoMetrics reads provider-reported metrics for each video that had
// activity in the last 30 days. averageViewPercentage is average percentage
// watched, not a full-watch completion rate.
func (y *YouTubeAnalyticsSource) FetchVideoMetrics(ctx context.Context, acct SourceAccount) ([]VideoMetric, error) {
	if acct.YoutubeChannel == "" {
		return nil, fmt.Errorf("%w: chưa gắn kênh YouTube cho tài khoản %s", ErrNotConnected, acct.Username)
	}
	if !strings.HasPrefix(strings.TrimSpace(acct.YoutubeChannel), "UC") {
		return nil, fmt.Errorf("%w: YouTube Analytics cần Channel ID bắt đầu bằng UC; @handle hiện chỉ dùng được với YouTube Data API", ErrNotConnected)
	}
	if len(acct.PublishedVideoIDs) == 0 {
		return nil, nil
	}
	if y.TokenFor == nil {
		return nil, fmt.Errorf("%w: YouTube Analytics chưa cấu hình (thiếu token OAuth)", ErrNotConnected)
	}
	token, err := y.TokenFor(acct.Username)
	if err != nil || token == "" {
		return nil, fmt.Errorf("%w: không lấy được access token YouTube cho %s: %v", ErrNotConnected, acct.Username, err)
	}
	end := time.Now().UTC()
	q := url.Values{}
	q.Set("ids", "channel=="+acct.YoutubeChannel)
	q.Set("startDate", end.AddDate(0, 0, -29).Format("2006-01-02"))
	q.Set("endDate", end.Format("2006-01-02"))
	q.Set("dimensions", "video")
	q.Set("metrics", "views,likes,comments,shares,averageViewPercentage,subscribersGained")
	q.Set("maxResults", "200")
	videoIDs := uniqueYouTubeVideoIDs(acct.PublishedVideoIDs)
	if len(videoIDs) == 0 {
		return nil, nil
	}
	// `video` is accepted as a grouping dimension when the request also filters
	// to explicit video IDs. A bare `dimensions=video` query is unsupported.
	const maxVideoFilterIDs = 500
	var out []VideoMetric
	for offset := 0; offset < len(videoIDs); offset += maxVideoFilterIDs {
		end := min(offset+maxVideoFilterIDs, len(videoIDs))
		q.Set("filters", "video=="+strings.Join(videoIDs[offset:end], ","))
		for start := 1; ; {
			q.Set("startIndex", strconv.Itoa(start))
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, y.base()+"/v2/reports?"+q.Encode(), nil)
			if err != nil {
				return nil, err
			}
			req.Header.Set("Authorization", "Bearer "+token)
			resp, err := y.httpClient().Do(req)
			if err != nil {
				return nil, fmt.Errorf("youtube analytics video report: lỗi mạng: %w", err)
			}
			body, readErr := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
			resp.Body.Close()
			if readErr != nil {
				return nil, readErr
			}
			if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
				return nil, fmt.Errorf("youtube analytics video report: token hết hạn hoặc thiếu quyền yt-analytics.readonly")
			}
			if resp.StatusCode != http.StatusOK {
				return nil, fmt.Errorf("youtube analytics video report: HTTP %d: %.200s", resp.StatusCode, body)
			}
			var report struct {
				ColumnHeaders []struct {
					Name string `json:"name"`
				} `json:"columnHeaders"`
				Rows [][]any `json:"rows"`
			}
			if err := json.Unmarshal(body, &report); err != nil {
				return nil, fmt.Errorf("youtube analytics video report: parse lỗi: %w", err)
			}
			columns := make(map[string]int, len(report.ColumnHeaders))
			for i, header := range report.ColumnHeaders {
				columns[header.Name] = i
			}
			for _, row := range report.Rows {
				idAt, ok := columns["video"]
				if !ok || idAt >= len(row) {
					continue
				}
				videoID, _ := row[idAt].(string)
				if videoID == "" {
					continue
				}
				metric := VideoMetric{PlatformVideoID: videoID, TakenAt: time.Now()}
				metric.Views = reportInt(row, columns, "views")
				metric.Likes = reportInt(row, columns, "likes")
				metric.Comments = reportInt(row, columns, "comments")
				metric.Shares = reportInt(row, columns, "shares")
				metric.SubscribersGained = reportInt(row, columns, "subscribersGained")
				metric.AverageViewPercentage = reportFloat(row, columns, "averageViewPercentage")
				out = append(out, metric)
			}
			if len(report.Rows) < 200 {
				break
			}
			start += len(report.Rows)
		}
	}
	return out, nil
}

func uniqueYouTubeVideoIDs(values []string) []string {
	seen := make(map[string]bool, len(values))
	ids := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if len(value) != 11 || seen[value] {
			continue
		}
		valid := true
		for _, c := range value {
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-') {
				valid = false
				break
			}
		}
		if valid {
			seen[value] = true
			ids = append(ids, value)
		}
	}
	return ids
}

func reportFloat(row []any, columns map[string]int, name string) *float64 {
	i, ok := columns[name]
	if !ok || i >= len(row) || row[i] == nil {
		return nil
	}
	v := numOf(row[i])
	return &v
}

func reportInt(row []any, columns map[string]int, name string) *int64 {
	v := reportFloat(row, columns, name)
	if v == nil {
		return nil
	}
	n := int64(*v)
	return &n
}

// report chạy 1 truy vấn Analytics `days` ngày gần nhất. dimension == ""
// → map metric→giá trị (hàng tổng); dimension != "" → map giá trị
// dimension→metric (vd "SHORTS"→views).
func (y *YouTubeAnalyticsSource) report(ctx context.Context, token, channel string, days int, metric, dimension string) (map[string]float64, error) {
	end := time.Now().UTC()
	q := url.Values{}
	q.Set("ids", "channel=="+channel)
	q.Set("startDate", end.AddDate(0, 0, -(days-1)).Format("2006-01-02"))
	q.Set("endDate", end.Format("2006-01-02"))
	q.Set("metrics", metric)
	if dimension != "" {
		q.Set("dimensions", dimension)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, y.base()+"/v2/reports?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := y.httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var rep struct {
		ColumnHeaders []struct {
			Name string `json:"name"`
		} `json:"columnHeaders"`
		Rows [][]any `json:"rows"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&rep); err != nil {
		return nil, err
	}
	out := map[string]float64{}
	if dimension == "" {
		if len(rep.Rows) > 0 {
			for i, h := range rep.ColumnHeaders {
				if i < len(rep.Rows[0]) {
					out[h.Name] = numOf(rep.Rows[0][i])
				}
			}
		}
		return out, nil
	}
	for _, row := range rep.Rows {
		if len(row) >= 2 {
			if k, ok := row[0].(string); ok {
				out[k] = numOf(row[1])
			}
		}
	}
	return out, nil
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
