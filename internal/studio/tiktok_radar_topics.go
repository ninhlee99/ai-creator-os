package studio

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// DefaultTikTokTopics chứa các chủ đề mặc định thịnh hành tại Việt Nam.
var DefaultTikTokTopics = []string{
	"Xu hướng Việt Nam",
	"Nhảy trend",
	"Thời trang",
	"Làm đẹp",
	"Review ẩm thực",
}

// ListTikTokRadarTopics trả về danh sách các chủ đề TikTok đang theo dõi.
func (s *Studio) ListTikTokRadarTopics(ctx context.Context) ([]string, error) {
	if s == nil || s.db == nil {
		return DefaultTikTokTopics, nil
	}
	rows, err := s.db.QueryContext(ctx, `SELECT name FROM studio_tiktok_radar_topics ORDER BY id ASC`)
	if err != nil {
		return DefaultTikTokTopics, nil
	}
	defer rows.Close()

	var topics []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err == nil && strings.TrimSpace(name) != "" {
			topics = append(topics, strings.TrimSpace(name))
		}
	}
	if len(topics) == 0 {
		return DefaultTikTokTopics, nil
	}
	return topics, nil
}

// AddTikTokRadarTopic thêm một chủ đề mới và quét ngay 5 video hot nhất.
func (s *Studio) AddTikTokRadarTopic(ctx context.Context, topic string) error {
	topic = strings.TrimSpace(topic)
	if topic == "" {
		return fmt.Errorf("chủ đề không được để trống")
	}
	if len([]rune(topic)) > 100 {
		return fmt.Errorf("tên chủ đề tối đa 100 ký tự")
	}
	if s == nil || s.db == nil {
		return fmt.Errorf("Studio chưa sẵn sàng")
	}

	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx, `INSERT INTO studio_tiktok_radar_topics(name, created_at) VALUES(?, ?) ON CONFLICT(name) DO NOTHING`, topic, now)
	if err != nil {
		return fmt.Errorf("không thêm được chủ đề: %w", err)
	}

	// Quét ngay 5 video hot nhất cho chủ đề này
	_, _ = s.CollectTikTokRadarVideosForTopic(ctx, topic)
	return nil
}

// DeleteTikTokRadarTopic xóa một chủ đề và danh sách video ánh xạ của nó.
func (s *Studio) DeleteTikTokRadarTopic(ctx context.Context, topic string) error {
	topic = strings.TrimSpace(topic)
	if topic == "" {
		return nil
	}
	if s == nil || s.db == nil {
		return fmt.Errorf("Studio chưa sẵn sàng")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM studio_tiktok_radar_topics WHERE name=?`, topic); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM studio_tiktok_radar_topic_videos WHERE topic=?`, topic); err != nil {
		return err
	}
	return tx.Commit()
}

// GetTikTokRadarVideosForTopic lấy 5 video hot nhất cho chủ đề đã lưu trong DB (realtime, không giật lag).
// Nếu chưa có trong DB, sẽ quét trực tiếp từ TikTok.
func (s *Studio) GetTikTokRadarVideosForTopic(ctx context.Context, topic string) ([]TikTokTrendVideo, error) {
	topic = strings.TrimSpace(topic)
	if topic == "" {
		topic = "Xu hướng Việt Nam"
	}
	if s == nil || s.db == nil {
		return s.CollectTikTokRadarVideosForTopic(ctx, topic)
	}

	type videoRow struct {
		rank            int
		payload         string
		danceVerified   bool
		danceVerifiedAt string
	}
	var rowsData []videoRow

	rows, err := s.db.QueryContext(ctx, `
		SELECT tv.rank, v.payload, v.dance_verified, v.dance_verified_at
		FROM studio_tiktok_radar_topic_videos tv
		JOIN studio_tiktok_radar_videos v ON tv.video_id = v.video_id
		WHERE tv.topic=?
		ORDER BY tv.rank ASC
		LIMIT ?
	`, topic, tikTokRadarFeedLimit)
	if err == nil {
		for rows.Next() {
			var r videoRow
			var danceVerifiedInt int
			if err := rows.Scan(&r.rank, &r.payload, &danceVerifiedInt, &r.danceVerifiedAt); err == nil {
				r.danceVerified = danceVerifiedInt != 0
				rowsData = append(rowsData, r)
			}
		}
		rows.Close()
	}

	if len(rowsData) > 0 {
		var videos []TikTokTrendVideo
		for _, r := range rowsData {
			var v TikTokTrendVideo
			if json.Unmarshal([]byte(r.payload), &v) == nil {
				v.Rank = r.rank
				v.DanceVerified = r.danceVerified
				v.DanceVerifiedAt = r.danceVerifiedAt
				_ = s.enrichTikTokRadarMetrics(ctx, &v)
				videos = append(videos, v)
			}
		}
		if len(videos) > 0 {
			sortTikTokRadarVideosByPopularity(videos)
			if len(videos) > tikTokRadarFeedLimit {
				videos = videos[:tikTokRadarFeedLimit]
			}
			for i := range videos {
				videos[i].Rank = i + 1
			}
			return videos, nil
		}
	}

	// Nếu chưa có video nào được lưu cho chủ đề này, quét từ TikTok
	return s.CollectTikTokRadarVideosForTopic(ctx, topic)
}

// CollectTikTokRadarVideosForTopic quét trực tiếp từ TikTok, lưu kết quả và cập nhật 5 video hot nhất cho chủ đề.
func (s *Studio) CollectTikTokRadarVideosForTopic(ctx context.Context, topic string) ([]TikTokTrendVideo, error) {
	topic = strings.TrimSpace(topic)
	if topic == "" {
		topic = "Xu hướng Việt Nam"
	}
	videos, err := s.SearchTikTokRadarVideos(ctx, topic)
	if err != nil {
		return nil, err
	}
	sortTikTokRadarVideosByPopularity(videos)
	if len(videos) > tikTokRadarFeedLimit {
		videos = videos[:tikTokRadarFeedLimit]
	}
	for i := range videos {
		videos[i].Rank = i + 1
	}

	if s != nil && s.db != nil && len(videos) > 0 {
		_ = s.saveTopicVideos(ctx, topic, videos)
	}
	return videos, nil
}

func (s *Studio) saveTopicVideos(ctx context.Context, topic string, videos []TikTokTrendVideo) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM studio_tiktok_radar_topic_videos WHERE topic=?`, topic); err != nil {
		return err
	}
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO studio_tiktok_radar_topic_videos(topic, video_id, rank, updated_at) VALUES(?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	now := time.Now().UTC().Format(time.RFC3339)
	for i, v := range videos {
		if strings.TrimSpace(v.VideoID) == "" {
			continue
		}
		if _, err := stmt.ExecContext(ctx, topic, v.VideoID, i+1, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}
