package studio

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"
)

// TikTokPublication links a completed Studio video to its public TikTok post.
type TikTokPublication struct {
	JobID           string
	AccountID       int64
	PlatformVideoID string
	VideoURL        string
	Title           string
	Kind            string
	LinkedAt        string
}

// LinkTikTokPublishedVideo stores the public video URL for a completed
// affiliate or viral job. Account ownership comes from the saved job params.
func (s *Studio) LinkTikTokPublishedVideo(jobID, rawURL string) (TikTokPublication, error) {
	job, ok := s.GetJob(strings.TrimSpace(jobID))
	if !ok {
		return TikTokPublication{}, fmt.Errorf("không tìm thấy job Studio")
	}
	if job.Kind != KindAffiliate && job.Kind != KindViral {
		return TikTokPublication{}, fmt.Errorf("chỉ liên kết video từ job Affiliate hoặc TikTok Viral")
	}
	if job.Status != StatusDone || !isTikTokVideoOutput(job.Output) {
		return TikTokPublication{}, fmt.Errorf("job cần hoàn tất và có file video trước khi liên kết")
	}

	accountID, err := jobAccountID(job)
	if err != nil || accountID <= 0 {
		return TikTokPublication{}, fmt.Errorf("job chưa lưu kênh TikTok đích")
	}
	videoID := TikTokVideoIDFromURL(rawURL)
	if videoID == "" {
		return TikTokPublication{}, fmt.Errorf("dán URL video TikTok đầy đủ có dạng /video/123…")
	}
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return TikTokPublication{}, fmt.Errorf("URL TikTok không hợp lệ")
	}
	canonicalURL := "https://www.tiktok.com" + parsed.EscapedPath()
	now := time.Now().UTC().Format(time.RFC3339)

	tx, err := s.db.Begin()
	if err != nil {
		return TikTokPublication{}, err
	}
	defer tx.Rollback()
	var existingJobID string
	err = tx.QueryRow(`SELECT job_id FROM studio_tiktok_publications WHERE account_id=? AND platform_video_id=?`, accountID, videoID).Scan(&existingJobID)
	if err != nil && err != sql.ErrNoRows {
		return TikTokPublication{}, err
	}
	if existingJobID != "" && existingJobID != job.ID {
		return TikTokPublication{}, fmt.Errorf("video TikTok này đã được liên kết với job khác")
	}
	if _, err := tx.Exec(`INSERT INTO studio_tiktok_publications(job_id,account_id,platform_video_id,video_url,linked_at)
		VALUES(?,?,?,?,?) ON CONFLICT(job_id) DO UPDATE SET
		account_id=excluded.account_id, platform_video_id=excluded.platform_video_id,
		video_url=excluded.video_url, linked_at=excluded.linked_at`,
		job.ID, accountID, videoID, canonicalURL, now); err != nil {
		return TikTokPublication{}, err
	}
	if err := tx.Commit(); err != nil {
		return TikTokPublication{}, err
	}
	return TikTokPublication{JobID: job.ID, AccountID: accountID, PlatformVideoID: videoID,
		VideoURL: canonicalURL, Title: job.Title, Kind: job.Kind, LinkedAt: now}, nil
}

func jobAccountID(job Job) (int64, error) {
	if job.Params == "" {
		return 0, fmt.Errorf("thiếu params")
	}
	switch job.Kind {
	case KindAffiliate:
		var params AffiliateParams
		if err := json.Unmarshal([]byte(job.Params), &params); err != nil {
			return 0, err
		}
		return params.AccountID, nil
	case KindViral:
		var params ViralParams
		if err := json.Unmarshal([]byte(job.Params), &params); err != nil {
			return 0, err
		}
		return params.AccountID, nil
	default:
		return 0, fmt.Errorf("loại job không hỗ trợ")
	}
}

func isTikTokVideoOutput(output string) bool {
	switch strings.ToLower(filepath.Ext(strings.TrimSpace(output))) {
	case ".mp4", ".mov", ".webm":
		return true
	default:
		return false
	}
}

// TikTokPublications returns linked Studio jobs for one TikTok account.
func (s *Studio) TikTokPublications(accountID int64, limit int) ([]TikTokPublication, error) {
	if accountID <= 0 {
		return nil, fmt.Errorf("studio: account ID không hợp lệ")
	}
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := s.db.Query(`SELECT p.job_id,p.account_id,p.platform_video_id,p.video_url,
		COALESCE(j.title,''),COALESCE(j.kind,''),p.linked_at
		FROM studio_tiktok_publications p LEFT JOIN studio_jobs j ON j.id=p.job_id
		WHERE p.account_id=? ORDER BY p.linked_at DESC LIMIT ?`, accountID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var publications []TikTokPublication
	for rows.Next() {
		var publication TikTokPublication
		if err := rows.Scan(&publication.JobID, &publication.AccountID, &publication.PlatformVideoID,
			&publication.VideoURL, &publication.Title, &publication.Kind, &publication.LinkedAt); err != nil {
			return nil, err
		}
		publications = append(publications, publication)
	}
	return publications, rows.Err()
}

// TikTokPublicationForJob returns the current public-video association, if any.
func (s *Studio) TikTokPublicationForJob(jobID string) (TikTokPublication, bool) {
	var publication TikTokPublication
	err := s.db.QueryRow(`SELECT p.job_id,p.account_id,p.platform_video_id,p.video_url,
		COALESCE(j.title,''),COALESCE(j.kind,''),p.linked_at
		FROM studio_tiktok_publications p LEFT JOIN studio_jobs j ON j.id=p.job_id
		WHERE p.job_id=?`, strings.TrimSpace(jobID)).Scan(
		&publication.JobID, &publication.AccountID, &publication.PlatformVideoID,
		&publication.VideoURL, &publication.Title, &publication.Kind, &publication.LinkedAt)
	return publication, err == nil
}
