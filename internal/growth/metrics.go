package growth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/publishers"
)

// ErrNotConnected marks "this metric source is not wired up" — a normal,
// honest state (missing key, missing channel mapping, platform gate). The
// sync layer reports it to the UI verbatim; it never fabricates numbers.
var ErrNotConnected = errors.New("chưa kết nối")

// SourceAccount is the account view a metric source needs.
type SourceAccount struct {
	ID       int64
	Username string
}

// MetricsSource reads REAL metrics from one provider API.
type MetricsSource interface {
	// Name is the snapshot source value.
	Name() string
	// Fetch reads the account's current metrics. Implementations must be
	// fail-closed: not configured => ErrNotConnected and zero network use.
	Fetch(ctx context.Context, acct SourceAccount) (*Snapshot, error)
}

// VideoMetricsSource reads real metrics for individual published videos.
// Sources without a provider endpoint for this data must not implement it.
type VideoMetricsSource interface {
	FetchVideoMetrics(ctx context.Context, acct SourceAccount) ([]VideoMetric, error)
}

// VideoMetric is one provider's latest reading for a published video.
// Nil means provider did not return that metric; it is never converted to 0.
type VideoMetric struct {
	PlatformVideoID       string
	Title                 string
	ShareURL              string
	PublishedAt           time.Time
	TakenAt               time.Time
	Views                 *int64
	Likes                 *int64
	Comments              *int64
	Shares                *int64
	Saves                 *int64
	AverageViewPercentage *float64 // provider's average percentage watched, 0–100+.
	SubscribersGained     *int64
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

// RecordVideoMetrics fetches and appends real per-video readings. No rows are
// written if the provider fetch fails.
func RecordVideoMetrics(ctx context.Context, store *Store, src VideoMetricsSource, acct SourceAccount) (int, error) {
	metrics, err := src.FetchVideoMetrics(ctx, acct)
	if err != nil {
		return 0, err
	}
	if err := store.InsertVideoMetricSnapshots(acct.ID, metrics); err != nil {
		return 0, err
	}
	return len(metrics), nil
}

// -------------------------------------------------------------- TikTok

// TikTokSource reads creator-authorized profile and public-video counters
// through TikTok Display API. Missing scopes and unavailable fields stay
// unknown; this source never scrapes TikTok pages.
type TikTokSource struct{}

func (TikTokSource) Name() string { return "tiktok_api" }

func (TikTokSource) Fetch(ctx context.Context, acct SourceAccount) (*Snapshot, error) {
	if acct.Username == "" {
		return nil, fmt.Errorf("%w: thiếu tên kênh TikTok", ErrNotConnected)
	}
	client := publishers.NewTikTokPublisher(acct.Username)
	if !client.IsConfigured() {
		return nil, fmt.Errorf("%w: %s chưa kết nối TikTok OAuth", ErrNotConnected, acct.Username)
	}
	stats, err := client.DisplayUserStats(ctx)
	if err != nil {
		return nil, fmt.Errorf("TikTok profile metrics: %w; nếu thiếu quyền, kết nối lại kênh để cấp user.info.stats", err)
	}
	if stats.Followers == nil && stats.Videos == nil && stats.Likes == nil {
		return nil, fmt.Errorf("%w: TikTok không trả profile statistics; cần scope user.info.stats", ErrNotConnected)
	}
	extra := map[string]any{}
	if stats.Likes != nil {
		extra["likes_count"] = *stats.Likes
	}
	return &Snapshot{
		AccountID: acct.ID,
		TakenAt:   time.Now().UTC(),
		Source:    "tiktok_api",
		Followers: stats.Followers,
		Videos:    stats.Videos,
		Extra:     extra,
	}, nil
}

func (TikTokSource) FetchVideoMetrics(ctx context.Context, acct SourceAccount) ([]VideoMetric, error) {
	if acct.Username == "" {
		return nil, fmt.Errorf("%w: thiếu tên kênh TikTok", ErrNotConnected)
	}
	client := publishers.NewTikTokPublisher(acct.Username)
	if !client.IsConfigured() {
		return nil, fmt.Errorf("%w: %s chưa kết nối TikTok OAuth", ErrNotConnected, acct.Username)
	}
	videos, err := client.DisplayVideoMetrics(ctx, 60)
	if err != nil {
		return nil, fmt.Errorf("TikTok video metrics: %w; nếu thiếu quyền, kết nối lại kênh để cấp video.list", err)
	}
	takenAt := time.Now().UTC()
	out := make([]VideoMetric, 0, len(videos))
	for _, video := range videos {
		out = append(out, VideoMetric{
			PlatformVideoID: video.ID,
			Title:           video.Title,
			ShareURL:        video.ShareURL,
			PublishedAt:     video.PublishedAt,
			TakenAt:         takenAt,
			Views:           video.Views,
			Likes:           video.Likes,
			Comments:        video.Comments,
			Shares:          video.Shares,
		})
	}
	return out, nil
}
