package growth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ErrNotConnected marks "this metric source is not wired up" — a normal,
// honest state (missing key, missing channel mapping, platform gate). The
// sync layer reports it to the UI verbatim; it never fabricates numbers.
var ErrNotConnected = errors.New("chưa kết nối")

// SourceAccount is the account view a metric source needs.
type SourceAccount struct {
	ID             int64
	Username       string
	YoutubeChannel string // channel id (UC...) or @handle; "" = not mapped
}

// MetricsSource reads REAL metrics from one provider API.
type MetricsSource interface {
	// Name is the snapshot `source` value: youtube_api | tiktok_api.
	Name() string
	// Fetch reads the account's current metrics. Implementations must be
	// fail-closed: not configured => ErrNotConnected and zero network use.
	Fetch(ctx context.Context, acct SourceAccount) (*Snapshot, error)
}

// RecordSnapshot fetches from src and appends the reading. It returns the
// stored snapshot, or the fetch error (nothing is written on error).
func RecordSnapshot(ctx context.Context, store *Store, src MetricsSource, acct SourceAccount) (*Snapshot, error) {
	snap, err := src.Fetch(ctx, acct)
	if err != nil {
		return nil, err
	}
	snap.AccountID = acct.ID
	snap.Source = src.Name()
	if snap.TakenAt.IsZero() {
		snap.TakenAt = time.Now()
	}
	if err := store.InsertSnapshot(*snap); err != nil {
		return nil, err
	}
	return snap, nil
}

// ------------------------------------------------------------- YouTube

// YouTubeSource reads channel statistics from the official YouTube Data
// API v3 (channels.list). It needs an API key (Settings: YOUTUBE_API_KEY)
// and the account's channel mapping (account detail: Kênh YouTube).
//
// Honest limits: Data API gives lifetime totals, not 30-day views or watch
// hours — those come from YouTubeAnalyticsSource (Đợt H3, OAuth-only), so
// Views30d stays unset until the Analytics token/scope is connected.
type YouTubeSource struct {
	APIKey  string
	BaseURL string // default https://www.googleapis.com
	Client  *http.Client
}

// NewYouTubeSource builds the source; empty key = not connected.
func NewYouTubeSource(apiKey string) *YouTubeSource {
	return &YouTubeSource{APIKey: apiKey}
}

func (y *YouTubeSource) Name() string { return "youtube_api" }

func (y *YouTubeSource) httpClient() *http.Client {
	if y.Client != nil {
		return y.Client
	}
	return &http.Client{Timeout: 20 * time.Second}
}

func (y *YouTubeSource) base() string {
	if y.BaseURL != "" {
		return strings.TrimSuffix(y.BaseURL, "/")
	}
	return "https://www.googleapis.com"
}

func (y *YouTubeSource) Fetch(ctx context.Context, acct SourceAccount) (*Snapshot, error) {
	if strings.TrimSpace(y.APIKey) == "" {
		return nil, fmt.Errorf("%w: YouTube (chưa nhập YOUTUBE_API_KEY trong Cài đặt)", ErrNotConnected)
	}
	ch := strings.TrimSpace(acct.YoutubeChannel)
	if ch == "" {
		return nil, fmt.Errorf("%w: YouTube (tài khoản %s chưa gắn kênh — nhập ở trang chi tiết tài khoản)", ErrNotConnected, acct.Username)
	}
	param := "id=" + ch
	if !strings.HasPrefix(ch, "UC") {
		if !strings.HasPrefix(ch, "@") {
			ch = "@" + ch
		}
		param = "forHandle=" + ch
	}
	url := fmt.Sprintf("%s/youtube/v3/channels?part=statistics&%s&key=%s", y.base(), param, y.APIKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := y.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("youtube api: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("youtube api: HTTP %d: %s", resp.StatusCode, snippet(body))
	}
	var parsed struct {
		Items []struct {
			Statistics struct {
				ViewCount             string `json:"viewCount"`
				SubscriberCount       string `json:"subscriberCount"`
				VideoCount            string `json:"videoCount"`
				HiddenSubscriberCount bool   `json:"hiddenSubscriberCount"`
			} `json:"statistics"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("youtube api: bad response: %w", err)
	}
	if len(parsed.Items) == 0 {
		return nil, fmt.Errorf("youtube api: không tìm thấy kênh %q", acct.YoutubeChannel)
	}
	st := parsed.Items[0].Statistics
	snap := &Snapshot{Extra: map[string]any{}}
	if n, err := strconv.ParseInt(st.ViewCount, 10, 64); err == nil {
		snap.Extra["views_total"] = n
	}
	if !st.HiddenSubscriberCount {
		if n, err := strconv.ParseInt(st.SubscriberCount, 10, 64); err == nil {
			snap.Followers = &n
		}
	}
	if n, err := strconv.ParseInt(st.VideoCount, 10, 64); err == nil {
		snap.Videos = &n
	}
	return snap, nil
}

// -------------------------------------------------------------- TikTok

// TikTokSource is the honest MVP stub: TikTok Display API metrics need a
// per-account OAuth grant plus the Direct Post audit before any public
// numbers exist (docs/CHANNEL_GROWTH.md §6.3). Until then this source is
// never connected — the app does not scrape TikTok, ever.
type TikTokSource struct{}

func (TikTokSource) Name() string { return "tiktok_api" }

func (TikTokSource) Fetch(_ context.Context, acct SourceAccount) (*Snapshot, error) {
	return nil, fmt.Errorf("%w: TikTok (Display API cần OAuth từng tài khoản + audit Direct Post; hiện chỉ đăng nháp)", ErrNotConnected)
}

func snippet(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 200 {
		s = s[:200] + "…"
	}
	return s
}
