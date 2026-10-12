package reup

import (
	"database/sql"
	"fmt"
	"strings"
)

type Video struct {
	ID            int64
	SourceID      int64
	DouyinID      string
	URL           string
	FilePath      string
	SHA256        string
	DurationSec   float64
	Resolution    string
	PlayCount     int64
	Author        string
	Title         string
	WatermarkFree bool
	Via           string // "ytdlp" | "tikwm"
	Status        string
	FailReason    string
	QCMethod      string
	DownloadedAt  string
	CreatedAt     string
}

// StatusLabel nhãn tiếng Việt trung thực cho trạng thái.
func (v Video) StatusLabel() string {
	switch v.Status {
	case StatusDownloaded:
		return "đã tải"
	case StatusFailed:
		return "tải lỗi"
	case StatusDownloading:
		return "đang tải"
	default:
		return "chờ tải"
	}
}

// WatermarkLabel nhãn trung thực về watermark (không đoán mò).
// Từ Đợt I mọi video tải về đều giữ watermark gốc (yt-dlp giữ nguyên;
// TikWM chỉ dùng link wmplay) — "không watermark" chỉ còn là giá trị
// lịch sử cho bản ghi cũ trước Đợt I.
func (v Video) WatermarkLabel() string {
	if v.Status != StatusDownloaded {
		return "—"
	}
	if v.WatermarkFree {
		return "không watermark"
	}
	if v.Via == "tikwm" {
		return "có watermark"
	}
	return "có thể có watermark"
}

// HasDouyinID kiểm tra video đã BIẾT trong kho (queued/downloading/
// downloaded) theo douyin_id — dùng cho dedupe trước tải và discover
// (không pick lại video đã biết). Video failed KHÔNG tính — được phép
// thử lại (Retry).
func (s *Store) HasDouyinID(douyinID string) bool {
	if douyinID == "" {
		return false
	}
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM reup_videos
		WHERE douyin_id=? AND status != 'failed'`, douyinID).Scan(&n); err != nil {
		return false
	}
	return n > 0
}

// HasSHA256 kiểm tra file trùng nội dung (dedupe sau tải).
func (s *Store) HasSHA256(sha string) bool {
	if sha == "" {
		return false
	}
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM reup_videos WHERE sha256=? AND status='downloaded'`,
		sha).Scan(&n); err != nil {
		return false
	}
	return n > 0
}

// QueueVideo ghi nhận video phát hiện được (chờ tải). Trùng douyin_id
// thì trả về bản đã có, không tạo trùng.
func (s *Store) QueueVideo(v Video) (Video, error) {
	if v.DouyinID != "" {
		if ex, err := s.GetByDouyinID(v.DouyinID); err == nil {
			return ex, nil
		}
	}
	if v.Status == "" {
		v.Status = StatusQueued
	}
	res, err := s.db.Exec(`INSERT INTO reup_videos
		(source_id, douyin_id, url, file_path, sha256, duration_sec, resolution,
		 play_count, author, title, watermark_free, via, status, fail_reason,
		 qc_method, downloaded_at, created_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		v.SourceID, v.DouyinID, v.URL, v.FilePath, v.SHA256, v.DurationSec, v.Resolution,
		v.PlayCount, v.Author, v.Title, boolInt(v.WatermarkFree), v.Via, v.Status,
		v.FailReason, v.QCMethod, v.DownloadedAt, nowUTC())
	if err != nil {
		return Video{}, fmt.Errorf("reup: xếp hàng video: %w", err)
	}
	v.ID, _ = res.LastInsertId()
	v.CreatedAt = nowUTC()
	return v, nil
}

// GetByDouyinID đọc video theo douyin_id.
func (s *Store) GetByDouyinID(douyinID string) (Video, error) {
	var v Video
	err := s.db.QueryRow(`SELECT id, source_id, douyin_id, url, file_path, sha256,
		duration_sec, resolution, play_count, author, title, watermark_free, via,
		status, fail_reason, qc_method, downloaded_at, created_at
		FROM reup_videos WHERE douyin_id=? ORDER BY id DESC LIMIT 1`, douyinID).
		Scan(&v.ID, &v.SourceID, &v.DouyinID, &v.URL, &v.FilePath, &v.SHA256,
			&v.DurationSec, &v.Resolution, &v.PlayCount, &v.Author, &v.Title,
			&v.WatermarkFree, &v.Via, &v.Status, &v.FailReason, &v.QCMethod,
			&v.DownloadedAt, &v.CreatedAt)
	if err != nil {
		return Video{}, err
	}
	return v, nil
}

// GetVideo đọc video theo id.
func (s *Store) GetVideo(id int64) (Video, error) {
	var v Video
	err := s.db.QueryRow(`SELECT id, source_id, douyin_id, url, file_path, sha256,
		duration_sec, resolution, play_count, author, title, watermark_free, via,
		status, fail_reason, qc_method, downloaded_at, created_at
		FROM reup_videos WHERE id=?`, id).
		Scan(&v.ID, &v.SourceID, &v.DouyinID, &v.URL, &v.FilePath, &v.SHA256,
			&v.DurationSec, &v.Resolution, &v.PlayCount, &v.Author, &v.Title,
			&v.WatermarkFree, &v.Via, &v.Status, &v.FailReason, &v.QCMethod,
			&v.DownloadedAt, &v.CreatedAt)
	if err != nil {
		return Video{}, fmt.Errorf("reup: đọc video #%d: %w", id, err)
	}
	return v, nil
}

// ListVideos liệt kê video (mới nhất trước, limit 0 = tất cả).
func (s *Store) ListVideos(limit int) ([]Video, error) {
	q := `SELECT id, source_id, douyin_id, url, file_path, sha256,
		duration_sec, resolution, play_count, author, title, watermark_free, via,
		status, fail_reason, qc_method, downloaded_at, created_at
		FROM reup_videos ORDER BY id DESC`
	if limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", limit)
	}
	rows, err := s.db.Query(q)
	if err != nil {
		return nil, fmt.Errorf("reup: liệt kê video: %w", err)
	}
	defer rows.Close()
	var out []Video
	for rows.Next() {
		var v Video
		if err := rows.Scan(&v.ID, &v.SourceID, &v.DouyinID, &v.URL, &v.FilePath,
			&v.SHA256, &v.DurationSec, &v.Resolution, &v.PlayCount, &v.Author,
			&v.Title, &v.WatermarkFree, &v.Via, &v.Status, &v.FailReason,
			&v.QCMethod, &v.DownloadedAt, &v.CreatedAt); err != nil {
			return nil, fmt.Errorf("reup: liệt kê video: %w", err)
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// VideoStats đếm nhanh theo trạng thái cho UI.
type VideoStats struct {
	Queued, Downloading, Downloaded, Failed int64
}

// Stats đếm video theo trạng thái.
func (s *Store) Stats() (VideoStats, error) {
	var st VideoStats
	rows, err := s.db.Query(`SELECT status, COUNT(*) FROM reup_videos GROUP BY status`)
	if err != nil {
		return st, fmt.Errorf("reup: thống kê: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var status string
		var n int64
		if err := rows.Scan(&status, &n); err != nil {
			return st, fmt.Errorf("reup: thống kê: %w", err)
		}
		switch status {
		case StatusQueued:
			st.Queued = n
		case StatusDownloading:
			st.Downloading = n
		case StatusDownloaded:
			st.Downloaded = n
		case StatusFailed:
			st.Failed = n
		}
	}
	return st, rows.Err()
}

// SetStatus đổi trạng thái video (+ lý do lỗi khi failed).
func (s *Store) SetStatus(id int64, status, failReason string) error {
	_, err := s.db.Exec(`UPDATE reup_videos SET status=?, fail_reason=? WHERE id=?`,
		status, failReason, id)
	if err != nil {
		return fmt.Errorf("reup: đổi trạng thái: %w", err)
	}
	return nil
}

// MarkDownloaded ghi nhận tải xong + QC đạt.
func (s *Store) MarkDownloaded(id int64, filePath, sha256, via string, watermarkFree bool,
	durationSec float64, resolution, qcMethod string) error {
	wf := 0
	if watermarkFree {
		wf = 1
	}
	_, err := s.db.Exec(`UPDATE reup_videos SET
		file_path=?, sha256=?, via=?, watermark_free=?, duration_sec=?,
		resolution=?, qc_method=?, status=?, fail_reason='', downloaded_at=?
		WHERE id=?`,
		filePath, sha256, via, wf, durationSec, resolution, qcMethod,
		StatusDownloaded, nowUTC(), id)
	if err != nil {
		return fmt.Errorf("reup: ghi nhận tải xong: %w", err)
	}
	return nil
}

// ------------------------------------------------------------------ helpers

func normKind(k string) string {
	if k == "hashtag" {
		return "hashtag"
	}
	return "user"
}

func trimWS(s string) string { return strings.TrimSpace(s) }

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// migrateV2 (Đợt E): reup_posts thêm cột cho transform + đăng + kill rule.
func migrateV2(db *sql.DB) error {
	for _, col := range []string{
		"ALTER TABLE reup_posts ADD COLUMN video_ids TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE reup_posts ADD COLUMN file_path TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE reup_posts ADD COLUMN fail_reason TEXT NOT NULL DEFAULT ''",
		// views = -1 nghĩa là CHƯA CÓ số liệu thật (không bịa 0).
		"ALTER TABLE reup_posts ADD COLUMN views INTEGER NOT NULL DEFAULT -1",
		"ALTER TABLE reup_posts ADD COLUMN metrics_at TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE reup_posts ADD COLUMN remote_id TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE reup_posts ADD COLUMN remote_url TEXT NOT NULL DEFAULT ''",
	} {
		if _, err := db.Exec(col); err != nil {
			return fmt.Errorf("reup: migrate v2: %w", err)
		}
	}
	// Backfill video_ids từ video_id cho bản ghi cũ.
	if _, err := db.Exec(`UPDATE reup_posts SET video_ids = CAST(video_id AS TEXT)
		WHERE video_ids = '' AND video_id != 0`); err != nil {
		return fmt.Errorf("reup: migrate v2 backfill: %w", err)
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_reup_posts_status
		ON reup_posts(status)`); err != nil {
		return fmt.Errorf("reup: migrate v2 index: %w", err)
	}
	return nil
}

// migrateV3 (Đợt V): reup_posts thêm updated_at để sweeper nhận ra bài kẹt
// ở trạng thái "posting" quá lâu (crash giữa upload và cập nhật trạng thái).
func migrateV3(db *sql.DB) error {
	if _, err := db.Exec(`ALTER TABLE reup_posts ADD COLUMN updated_at TEXT NOT NULL DEFAULT ''`); err != nil {
		return fmt.Errorf("reup: migrate v3: %w", err)
	}
	// Backfill: bài cũ lấy created_at làm mốc.
	if _, err := db.Exec(`UPDATE reup_posts SET updated_at = created_at WHERE updated_at = ''`); err != nil {
		return fmt.Errorf("reup: migrate v3 backfill: %w", err)
	}
	return nil
}

// ------------------------------------------------------------------ posts
// reup_posts: mỗi video (hoặc compilation) → transform → bài đăng per kênh.

// Trạng thái post.
const (
	PostPending      = "pending"      // chờ transform
	PostTransforming = "transforming" // đang transform
	PostTransformed  = "transformed"  // transform xong, chờ đăng
	PostPosting      = "posting"      // đang đăng
	PostPosted       = "posted"       // đã đăng
	PostFailed       = "failed"       // transform/đăng lỗi
)

// Post là một bài reup: transform của 1 video (mức 1) hoặc 3 video (mức 2)
// → đăng lên 1 kênh/nền tảng.
