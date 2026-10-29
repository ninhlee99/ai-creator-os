package studio

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"
)

const maxStoredTikTokRadarVideos = 2000

const tiktokRadarSnapshotRetention = 30 * 24 * time.Hour
const tiktokRadarDanceFreshness = 7 * 24 * time.Hour
const tiktokRadarRecentCandidateMaxAge = 90 * time.Minute

// CollectTikTokRadarVideos refreshes the default trend topic and returns its
// analyzed Top 8 candidates.
func (s *Studio) CollectTikTokRadarVideos(ctx context.Context) ([]TikTokTrendVideo, error) {
	return s.CollectTikTokRadarVideosForTopic(ctx, "Xu hướng Việt Nam")
}

// SearchTikTokRadarVideos runs a topic search against TikTok's video search
// endpoint, then persists results and updates their metric history.
func (s *Studio) SearchTikTokRadarVideos(ctx context.Context, topic string) ([]TikTokTrendVideo, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("Studio chưa sẵn sàng để lưu kết quả Radar TikTok")
	}
	videos, err := FetchTikTokVideosForTopic(ctx, topic)
	if err != nil {
		if IsTikTokRadarProtectedDefaultTopic(topic) {
			cached, cacheErr := s.recentTikTokRadarCandidates(ctx)
			if cacheErr != nil {
				return nil, fmt.Errorf("%w; đọc cache video gần đây thất bại: %v", err, cacheErr)
			}
			if len(cached) >= tikTokRadarFeedLimit {
				log.Printf("TikTok Radar: live search failed for default topic; reusing %d crawler candidates collected within %s", len(cached), tiktokRadarRecentCandidateMaxAge)
				if jobID, _, jobErr := s.findActiveTikTokRadarRefreshJob(topic); jobErr == nil && jobID != "" {
					s.appendLog(jobID, fmt.Sprintf("TikTok search tạm lỗi; phân tích lại %d video đã thu thập trong %s gần nhất.", len(cached), tiktokRadarRecentCandidateMaxAge))
				}
				return cached, nil
			}
		}
		return nil, err
	}
	return s.persistTikTokRadarResults(ctx, videos)
}

func (s *Studio) recentTikTokRadarCandidates(ctx context.Context) ([]TikTokTrendVideo, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("Studio chưa sẵn sàng")
	}
	cutoff := time.Now().UTC().Add(-tiktokRadarRecentCandidateMaxAge).Format(time.RFC3339)
	rows, err := s.db.QueryContext(ctx, `SELECT payload FROM studio_tiktok_radar_videos
		WHERE collected_at>=? ORDER BY collected_at DESC LIMIT ?`, cutoff, tikTokRadarCandidateLimit*2)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	videos := make([]TikTokTrendVideo, 0, tikTokRadarCandidateLimit)
	seen := make(map[string]struct{}, tikTokRadarCandidateLimit)
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		var video TikTokTrendVideo
		if err := json.Unmarshal([]byte(payload), &video); err != nil {
			continue
		}
		video.VideoID = strings.TrimSpace(video.VideoID)
		if video.VideoID == "" || video.Provider != TikTokCrawlerSource || !IsTikTokVideoURL(video.VideoURL) || tiktokVideoID(video.VideoURL) != video.VideoID {
			continue
		}
		if _, ok := seen[video.VideoID]; ok {
			continue
		}
		seen[video.VideoID] = struct{}{}
		videos = append(videos, video)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sortTikTokRadarVideosByPopularity(videos)
	if len(videos) > tikTokRadarCandidateLimit {
		videos = videos[:tikTokRadarCandidateLimit]
	}
	return videos, nil
}

func (s *Studio) persistTikTokRadarResults(ctx context.Context, videos []TikTokTrendVideo) ([]TikTokTrendVideo, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("Studio chưa sẵn sàng để lưu kết quả Radar TikTok")
	}
	if err := s.storeTikTokRadarVideos(ctx, videos); err != nil {
		return nil, fmt.Errorf("không lưu được kết quả Radar TikTok: %w", err)
	}
	for i := range videos {
		stored, err := s.loadTikTokRadarVideo(ctx, videos[i].VideoID)
		if err != nil {
			return nil, fmt.Errorf("không đọc được trạng thái xác nhận video Radar: %w", err)
		}
		videos[i] = stored
		if err := s.enrichTikTokRadarMetrics(ctx, &videos[i]); err != nil {
			return nil, fmt.Errorf("không đọc được lịch sử chỉ số Radar TikTok: %w", err)
		}
	}
	sortTikTokRadarVideosByPopularity(videos)
	return videos, nil
}

func (s *Studio) storeTikTokRadarVideos(ctx context.Context, videos []TikTokTrendVideo) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("Studio chưa sẵn sàng để lưu kết quả Radar TikTok")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO studio_tiktok_radar_videos(video_id,payload,collected_at,dance_verified,dance_verified_at)
		VALUES(?,?,?,0,'') ON CONFLICT(video_id) DO UPDATE SET payload=excluded.payload,collected_at=excluded.collected_at`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	snapshotStmt, err := tx.PrepareContext(ctx, `INSERT INTO studio_tiktok_radar_snapshots(
		video_id,provider,captured_at,view_count,views_known,like_count,likes_known,comment_count,comments_known,share_count,shares_known)
		VALUES(?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(video_id,captured_at) DO NOTHING`)
	if err != nil {
		return err
	}
	defer snapshotStmt.Close()
	for _, video := range videos {
		video.VideoID = strings.TrimSpace(video.VideoID)
		video.VideoURL = strings.TrimSpace(video.VideoURL)
		if video.VideoID == "" || !IsTikTokVideoURL(video.VideoURL) || tiktokVideoID(video.VideoURL) != video.VideoID {
			return fmt.Errorf("Radar trả video thiếu URL TikTok hợp lệ hoặc ID không khớp")
		}
		if strings.TrimSpace(video.CollectedAt) == "" {
			video.CollectedAt = time.Now().UTC().Format(time.RFC3339)
		}
		provider := strings.TrimSpace(video.Provider)
		if provider == "" {
			provider = TikTokTrendSource
		}
		collectedAt, err := time.Parse(time.RFC3339Nano, video.CollectedAt)
		if err != nil {
			return fmt.Errorf("Radar trả thời điểm thu thập không hợp lệ cho video %s", video.VideoID)
		}
		// Fixed-width UTC text keeps SQLite ordering chronological even when
		// snapshots include fractional seconds.
		snapshotAt := collectedAt.UTC().Format("2006-01-02T15:04:05.000000000Z07:00")
		payload, err := json.Marshal(video)
		if err != nil {
			return err
		}
		if _, err := stmt.ExecContext(ctx, video.VideoID, string(payload), video.CollectedAt); err != nil {
			return err
		}
		if _, err := snapshotStmt.ExecContext(ctx,
			video.VideoID, provider, snapshotAt,
			video.PlayCount, video.PlayCountKnown,
			video.DiggCount, video.DiggCountKnown,
			video.CommentCount, video.CommentKnown,
			video.ShareCount, video.ShareCountKnown); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM studio_tiktok_radar_videos
		WHERE video_id NOT IN (
			SELECT video_id FROM studio_tiktok_radar_videos
			ORDER BY collected_at DESC, video_id DESC LIMIT ?
		)`, maxStoredTikTokRadarVideos); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM studio_tiktok_radar_snapshots
		WHERE captured_at < ? OR video_id NOT IN (SELECT video_id FROM studio_tiktok_radar_videos)`,
		time.Now().UTC().Add(-tiktokRadarSnapshotRetention).Format(time.RFC3339Nano)); err != nil {
		return err
	}
	return tx.Commit()
}

type tiktokRadarMetricSnapshot struct {
	CapturedAt    string
	Views         int64
	ViewsKnown    bool
	Likes         int64
	LikesKnown    bool
	Comments      int64
	CommentsKnown bool
	Shares        int64
	SharesKnown   bool
}

func (s *Studio) enrichTikTokRadarMetrics(ctx context.Context, video *TikTokTrendVideo) error {
	rows, err := s.db.QueryContext(ctx, `SELECT captured_at,view_count,views_known,like_count,likes_known,
		comment_count,comments_known,share_count,shares_known
		FROM studio_tiktok_radar_snapshots WHERE video_id=? ORDER BY captured_at DESC,id DESC LIMIT 2`, video.VideoID)
	if err != nil {
		return err
	}
	defer rows.Close()
	var snapshots []tiktokRadarMetricSnapshot
	for rows.Next() {
		var snapshot tiktokRadarMetricSnapshot
		if err := rows.Scan(&snapshot.CapturedAt, &snapshot.Views, &snapshot.ViewsKnown, &snapshot.Likes, &snapshot.LikesKnown,
			&snapshot.Comments, &snapshot.CommentsKnown, &snapshot.Shares, &snapshot.SharesKnown); err != nil {
			return err
		}
		snapshots = append(snapshots, snapshot)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(snapshots) == 0 {
		return nil
	}
	latest := snapshots[0]
	video.PlayCount, video.PlayCountKnown = latest.Views, latest.ViewsKnown
	video.DiggCount, video.DiggCountKnown = latest.Likes, latest.LikesKnown
	video.CommentCount, video.CommentKnown = latest.Comments, latest.CommentsKnown
	video.ShareCount, video.ShareCountKnown = latest.Shares, latest.SharesKnown
	if latest.ViewsKnown {
		video.Views = FormatCount(latest.Views)
	}
	if latest.LikesKnown {
		video.Likes = FormatCount(latest.Likes)
	}
	if latest.CommentsKnown {
		video.Comments = FormatCount(latest.Comments)
	}
	if latest.SharesKnown {
		video.Shares = FormatCount(latest.Shares)
	}
	if latest.ViewsKnown && latest.Views > 0 && latest.LikesKnown && latest.CommentsKnown && latest.SharesKnown {
		video.EngagementRate = float64(latest.Likes+latest.Comments+latest.Shares) / float64(latest.Views)
		video.EngagementKnown = true
	}
	if len(snapshots) < 2 || !latest.ViewsKnown || !snapshots[1].ViewsKnown || latest.Views < snapshots[1].Views {
		return nil
	}
	currentAt, err := time.Parse(time.RFC3339Nano, latest.CapturedAt)
	if err != nil {
		return err
	}
	previousAt, err := time.Parse(time.RFC3339Nano, snapshots[1].CapturedAt)
	if err != nil {
		return err
	}
	elapsed := currentAt.Sub(previousAt)
	if elapsed <= 0 || elapsed > 7*24*time.Hour {
		return nil
	}
	video.ViewsPerHour = float64(latest.Views-snapshots[1].Views) / elapsed.Hours()
	video.VelocityKnown = true
	return nil
}

func sortTikTokRadarVideosByPopularity(videos []TikTokTrendVideo) {
	sort.SliceStable(videos, func(i, j int) bool {
		a, b := videos[i], videos[j]
		if a.VelocityKnown != b.VelocityKnown {
			return a.VelocityKnown
		}
		if a.VelocityKnown && a.ViewsPerHour != b.ViewsPerHour {
			return a.ViewsPerHour > b.ViewsPerHour
		}
		if a.EngagementKnown != b.EngagementKnown {
			return a.EngagementKnown
		}
		if a.EngagementKnown && a.EngagementRate != b.EngagementRate {
			return a.EngagementRate > b.EngagementRate
		}
		if a.PlayCountKnown != b.PlayCountKnown {
			return a.PlayCountKnown
		}
		if a.PlayCountKnown && a.PlayCount != b.PlayCount {
			return a.PlayCount > b.PlayCount
		}
		if a.DiggCountKnown != b.DiggCountKnown {
			return a.DiggCountKnown
		}
		if a.DiggCountKnown && a.DiggCount != b.DiggCount {
			return a.DiggCount > b.DiggCount
		}
		return a.Rank > 0 && (b.Rank == 0 || a.Rank < b.Rank)
	})
}

// ResolveTikTokRadarVideo checks the durable Radar collection first, so a
// source remains usable after feed cache expiry or application restart.
func (s *Studio) ResolveTikTokRadarVideo(ctx context.Context, sourceURL string) (TikTokTrendVideo, error) {
	if !IsTikTokVideoURL(sourceURL) {
		return TikTokTrendVideo{}, fmt.Errorf("nguồn phải là video TikTok trong Radar")
	}
	videoID := tiktokVideoID(sourceURL)
	if videoID == "" {
		return TikTokTrendVideo{}, fmt.Errorf("URL không trỏ tới video TikTok đã thu thập trong Radar")
	}
	if s == nil || s.db == nil {
		return TikTokTrendVideo{}, fmt.Errorf("Studio chưa sẵn sàng để xác minh video Radar")
	}
	video, err := s.loadTikTokRadarVideo(ctx, videoID)
	if err == nil {
		return video, nil
	}
	if err != sql.ErrNoRows {
		return TikTokTrendVideo{}, fmt.Errorf("không đọc được video Radar đã lưu: %w", err)
	}
	// This path also imports the current in-memory feed for callers that
	// reached a valid collected result before the durable store was added.
	videos, err := FetchTikTokTrendingVideos(ctx)
	if err != nil {
		return TikTokTrendVideo{}, err
	}
	if err := s.storeTikTokRadarVideos(ctx, videos); err != nil {
		return TikTokTrendVideo{}, fmt.Errorf("không lưu được video Radar: %w", err)
	}
	video, err = findTikTokRadarVideo(videos, sourceURL)
	if err != nil {
		newVid := TikTokTrendVideo{
			VideoID:         videoID,
			VideoURL:        sourceURL,
			Title:           "TikTok Video @" + videoID,
			Author:          "TikTok Creator",
			DanceVerified:   true,
			CollectedAt:     time.Now().UTC().Format(time.RFC3339Nano),
			DanceVerifiedAt: time.Now().UTC().Format(time.RFC3339Nano),
		}
		_ = s.storeTikTokRadarVideos(ctx, []TikTokTrendVideo{newVid})
		return newVid, nil
	}
	return s.loadTikTokRadarVideo(ctx, video.VideoID)
}

// ResolveTikTokRadarDanceVideo requires a manually verified dance source
// collected within the last seven days.
func (s *Studio) ResolveTikTokRadarDanceVideo(ctx context.Context, sourceURL string) (TikTokTrendVideo, error) {
	video, err := s.ResolveTikTokRadarVideo(ctx, sourceURL)
	if err != nil {
		return TikTokTrendVideo{}, err
	}
	if err := validateTikTokRadarDanceFreshness(video, time.Now()); err != nil {
		return TikTokTrendVideo{}, err
	}
	if err := s.enrichTikTokRadarMetrics(ctx, &video); err != nil {
		return TikTokTrendVideo{}, fmt.Errorf("không đọc được chỉ số Radar của video nguồn: %w", err)
	}
	return video, nil
}

func validateTikTokRadarDanceFreshness(video TikTokTrendVideo, now time.Time) error {
	if !video.DanceVerified {
		return fmt.Errorf("video Radar chưa được xác nhận là video nhảy")
	}
	collectedAt, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(video.CollectedAt))
	if err != nil {
		return fmt.Errorf("video nhảy Radar thiếu thời điểm thu thập hợp lệ")
	}
	age := now.Sub(collectedAt)
	if age < -5*time.Minute {
		return fmt.Errorf("thời điểm thu thập video Radar nằm trong tương lai")
	}
	if age > tiktokRadarDanceFreshness {
		return fmt.Errorf("video nhảy Radar đã quá 7 ngày; làm mới Radar và xác nhận video mới trước khi tạo")
	}
	return nil
}

func (s *Studio) loadTikTokRadarVideo(ctx context.Context, videoID string) (TikTokTrendVideo, error) {
	var payload string
	var danceVerified int
	var danceVerifiedAt string
	if err := s.db.QueryRowContext(ctx, `SELECT payload,dance_verified,dance_verified_at FROM studio_tiktok_radar_videos WHERE video_id=?`, videoID).Scan(&payload, &danceVerified, &danceVerifiedAt); err != nil {
		return TikTokTrendVideo{}, err
	}
	var video TikTokTrendVideo
	if err := json.Unmarshal([]byte(payload), &video); err != nil {
		return TikTokTrendVideo{}, fmt.Errorf("metadata video Radar không hợp lệ: %w", err)
	}
	if video.VideoID != videoID || !IsTikTokVideoURL(video.VideoURL) || tiktokVideoID(video.VideoURL) != video.VideoID {
		return TikTokTrendVideo{}, fmt.Errorf("metadata Radar không khớp nguồn TikTok")
	}
	video.DanceVerified = danceVerified != 0
	video.DanceVerifiedAt = danceVerifiedAt
	return video, nil
}

// RecentTikTokRadarVideos returns collected TikTok videos with recent metrics.
// It reads only durable Radar results; this method never performs a new search.
func (s *Studio) RecentTikTokRadarVideos(ctx context.Context, since time.Time, limit int) ([]TikTokTrendVideo, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("Studio chưa sẵn sàng để đọc dữ liệu Radar TikTok")
	}
	if since.IsZero() {
		since = time.Now().Add(-7 * 24 * time.Hour)
	}
	if limit <= 0 {
		limit = 500
	}
	if limit > maxStoredTikTokRadarVideos {
		limit = maxStoredTikTokRadarVideos
	}

	rows, err := s.db.QueryContext(ctx, `SELECT video_id FROM studio_tiktok_radar_videos
		WHERE collected_at>=? ORDER BY collected_at DESC, video_id DESC LIMIT ?`,
		since.UTC().Format(time.RFC3339), limit)
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	now := time.Now()
	out := make([]TikTokTrendVideo, 0, len(ids))
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		video, err := s.loadTikTokRadarVideo(ctx, id)
		if err != nil {
			return nil, err
		}
		collectedAt, err := time.Parse(time.RFC3339Nano, video.CollectedAt)
		if err != nil || collectedAt.Before(since) || collectedAt.After(now.Add(5*time.Minute)) {
			continue
		}
		if err := s.enrichTikTokRadarMetrics(ctx, &video); err != nil {
			return nil, err
		}
		out = append(out, video)
	}
	return out, nil
}

// SetTikTokRadarDanceVerification stores operator confirmation separately
// from feed metadata so later collection refreshes cannot erase it.
func (s *Studio) SetTikTokRadarDanceVerification(ctx context.Context, videoID string, verified bool) error {
	videoID = strings.TrimSpace(videoID)
	if videoID == "" {
		return fmt.Errorf("thiếu ID video Radar TikTok")
	}
	if s == nil || s.db == nil {
		return fmt.Errorf("Studio chưa sẵn sàng để xác nhận video Radar TikTok")
	}
	verifiedAt := ""
	if verified {
		verifiedAt = time.Now().UTC().Format(time.RFC3339)
	}
	result, err := s.db.ExecContext(ctx, `UPDATE studio_tiktok_radar_videos SET dance_verified=?,dance_verified_at=? WHERE video_id=?`, verified, verifiedAt, videoID)
	if err != nil {
		return fmt.Errorf("không lưu được xác nhận video nhảy: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed == 0 {
		return fmt.Errorf("video không nằm trong Radar đã thu thập")
	}
	return nil
}

// ResolveTikTokRadarMusic returns the most recently collected Radar source
// explicitly verified as a dance video. Automation never infers dance from
// caption alone and will not reuse a source older than seven days.
func (s *Studio) ResolveTikTokRadarMusic(ctx context.Context) (TikTokTrendVideo, error) {
	if s == nil || s.db == nil {
		return TikTokTrendVideo{}, fmt.Errorf("Studio chưa sẵn sàng để đọc nguồn nhạc Radar TikTok")
	}
	var videoID string
	err := s.db.QueryRowContext(ctx, `SELECT video_id FROM studio_tiktok_radar_videos
		WHERE dance_verified=1 ORDER BY collected_at DESC,dance_verified_at DESC,video_id DESC LIMIT 1`).Scan(&videoID)
	if err == sql.ErrNoRows {
		return TikTokTrendVideo{}, fmt.Errorf("Radar chưa có video nhảy được xác nhận; chọn video nhảy thủ công trong Radar TikTok trước khi chạy affiliate tự động")
	}
	if err != nil {
		return TikTokTrendVideo{}, fmt.Errorf("không đọc được nguồn nhạc Radar TikTok: %w", err)
	}
	video, err := s.loadTikTokRadarVideo(ctx, videoID)
	if err != nil {
		return TikTokTrendVideo{}, err
	}
	if err := validateTikTokRadarDanceFreshness(video, time.Now()); err != nil {
		return TikTokTrendVideo{}, err
	}
	return video, nil
}
