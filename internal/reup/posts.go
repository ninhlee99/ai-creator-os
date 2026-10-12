package reup

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"
)

type Post struct {
	ID             int64
	VideoIDs       []int64
	AccountRef     string
	Platform       string
	TransformLevel int
	Status         string
	FilePath       string
	FailReason     string
	Views          int64 // -1 = chưa có số liệu thật
	MetricsAt      string
	RemoteID       string
	RemoteURL      string
	PostedAt       string
	CreatedAt      string
}

// StatusLabel nhãn tiếng Việt.
func (p Post) StatusLabel() string {
	switch p.Status {
	case PostTransforming:
		return "đang transform"
	case PostTransformed:
		return "chờ đăng"
	case PostPosting:
		return "đang đăng"
	case PostPosted:
		return "đã đăng"
	case PostFailed:
		return "lỗi"
	default:
		return "chờ transform"
	}
}

// LevelLabel nhãn mức transform.
func (p Post) LevelLabel() string {
	if p.TransformLevel == Level2 {
		return "Mức 2"
	}
	return "Mức 1"
}

// ViewsLabel nhãn view trung thực: chưa có số liệu thì nói rõ.
func (p Post) ViewsLabel() string {
	if p.Views < 0 {
		return "chờ số liệu"
	}
	return fmt.Sprintf("%d", p.Views)
}

func parseVideoIDs(s string) []int64 {
	var out []int64
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		var id int64
		if _, err := fmt.Sscanf(part, "%d", &id); err == nil && id > 0 {
			out = append(out, id)
		}
	}
	return out
}

func joinVideoIDs(ids []int64) string {
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, fmt.Sprintf("%d", id))
	}
	return strings.Join(parts, ",")
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanPost(s rowScanner) (Post, error) {
	var p Post
	var videoIDs, accountRef, platform, status, filePath, failReason string
	var level int
	var views int64
	var metricsAt, remoteID, remoteURL, postedAt, createdAt string
	var videoID int64 // cột legacy, bỏ qua
	if err := s.Scan(&p.ID, &videoID, &accountRef, &platform, &level,
		&status, &postedAt, &createdAt,
		&videoIDs, &filePath, &failReason, &views, &metricsAt,
		&remoteID, &remoteURL); err != nil {
		return Post{}, err
	}
	p.VideoIDs = parseVideoIDs(videoIDs)
	p.AccountRef, p.Platform = accountRef, platform
	p.TransformLevel, p.Status = level, status
	p.FilePath, p.FailReason = filePath, failReason
	p.Views, p.MetricsAt = views, metricsAt
	p.RemoteID, p.RemoteURL = remoteID, remoteURL
	p.PostedAt, p.CreatedAt = postedAt, createdAt
	return p, nil
}

const postColumns = `id, video_id, account_ref, platform, transform_level,
	status, posted_at, created_at, video_ids, file_path, fail_reason,
	views, metrics_at, remote_id, remote_url`

// CreatePost tạo bài reup cho 1..n video ở mức transform cho trước.
func (s *Store) CreatePost(videoIDs []int64, level int) (Post, error) {
	if len(videoIDs) == 0 {
		return Post{}, fmt.Errorf("reup: tạo bài cần ít nhất 1 video")
	}
	if level != Level1 && level != Level2 {
		level = Level1
	}
	legacyID := videoIDs[0]
	res, err := s.db.Exec(`INSERT INTO reup_posts
		(video_id, account_ref, platform, transform_level, status,
		 posted_at, created_at, updated_at, video_ids, file_path, fail_reason,
		 views, metrics_at, remote_id, remote_url)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		legacyID, "", "", level, PostPending, "", nowUTC(), nowUTC(),
		joinVideoIDs(videoIDs), "", "", -1, "", "", "")
	if err != nil {
		return Post{}, fmt.Errorf("reup: tạo bài: %w", err)
	}
	id, _ := res.LastInsertId()
	return s.GetPost(id)
}

// GetPost đọc bài theo id.
func (s *Store) GetPost(id int64) (Post, error) {
	p, err := scanPost(s.db.QueryRow(`SELECT `+postColumns+` FROM reup_posts WHERE id=?`, id))
	if err != nil {
		return Post{}, fmt.Errorf("reup: đọc bài #%d: %w", id, err)
	}
	return p, nil
}

// ListPosts liệt kê bài (mới nhất trước, limit 0 = tất cả).
func (s *Store) ListPosts(limit int) ([]Post, error) {
	q := `SELECT ` + postColumns + ` FROM reup_posts ORDER BY id DESC`
	if limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", limit)
	}
	rows, err := s.db.Query(q)
	if err != nil {
		return nil, fmt.Errorf("reup: liệt kê bài: %w", err)
	}
	defer rows.Close()
	var out []Post
	for rows.Next() {
		p, err := scanPost(rows)
		if err != nil {
			return nil, fmt.Errorf("reup: liệt kê bài: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ListPostsByStatus liệt kê bài theo trạng thái.
func (s *Store) ListPostsByStatus(status string, limit int) ([]Post, error) {
	q := `SELECT ` + postColumns + ` FROM reup_posts WHERE status=? ORDER BY id ASC`
	if limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", limit)
	}
	rows, err := s.db.Query(q, status)
	if err != nil {
		return nil, fmt.Errorf("reup: liệt kê bài: %w", err)
	}
	defer rows.Close()
	var out []Post
	for rows.Next() {
		p, err := scanPost(rows)
		if err != nil {
			return nil, fmt.Errorf("reup: liệt kê bài: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ListPostedPosts liệt kê bài đã đăng (cũ nhất trước — cho kill rule).
func (s *Store) ListPostedPosts(limit int) ([]Post, error) {
	q := `SELECT ` + postColumns + ` FROM reup_posts WHERE status=? ORDER BY posted_at ASC, id ASC`
	if limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", limit)
	}
	rows, err := s.db.Query(q, PostPosted)
	if err != nil {
		return nil, fmt.Errorf("reup: liệt kê bài đã đăng: %w", err)
	}
	defer rows.Close()
	var out []Post
	for rows.Next() {
		p, err := scanPost(rows)
		if err != nil {
			return nil, fmt.Errorf("reup: liệt kê bài đã đăng: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// LivePostForVideo trả về bài còn hiệu lực (khác failed) chứa video —
// nil khi không có. Dùng để chặn transform trùng từ UI.
func (s *Store) LivePostForVideo(videoID int64) (*Post, error) {
	q := `SELECT ` + postColumns + ` FROM reup_posts
		WHERE (',' || video_ids || ',') LIKE '%,' || CAST(? AS TEXT) || ',%'
		AND status != ? ORDER BY id DESC LIMIT 1`
	row := s.db.QueryRow(q, videoID, PostFailed)
	p, err := scanPost(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reup: tìm bài của video: %w", err)
	}
	return &p, nil
}

// SetPostStatus đổi trạng thái bài (+ lý do khi failed).
func (s *Store) SetPostStatus(id int64, status, failReason string) error {
	_, err := s.db.Exec(`UPDATE reup_posts SET status=?, fail_reason=?, updated_at=? WHERE id=?`,
		status, failReason, nowUTC(), id)
	if err != nil {
		return fmt.Errorf("reup: đổi trạng thái bài: %w", err)
	}
	return nil
}

// SweepStuckPosting (Đợt V): bài kẹt ở "posting" quá maxAge (crash/kill -9
// giữa upload và cập nhật trạng thái) → trả về "transformed" để tick sau
// đăng lại. Trả về số bài đã reset.
func (s *Store) SweepStuckPosting(maxAge time.Duration) (int, error) {
	cutoff := time.Now().Add(-maxAge).UTC().Format(time.RFC3339)
	res, err := s.db.Exec(`UPDATE reup_posts SET status='transformed', updated_at=?
		WHERE status='posting' AND updated_at != '' AND updated_at < ?`,
		nowUTC(), cutoff)
	if err != nil {
		return 0, fmt.Errorf("reup: sweep posting kẹt: %w", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// SweepStuckTransforming (Đợt Y): bài kẹt ở "pending"/"transforming" quá
// maxAge (crash/kill giữa CreatePost và chạy transform) → "failed" với lý
// do rõ. Tick sau tự tạo bài mới cho video (NOT EXISTS loại bài failed).
// Trả về số bài đã reset.
func (s *Store) SweepStuckTransforming(maxAge time.Duration) (int, error) {
	cutoff := time.Now().Add(-maxAge).UTC().Format(time.RFC3339)
	res, err := s.db.Exec(`UPDATE reup_posts SET status='failed',
		fail_reason='treo quá lâu ở trạng thái transform (tự reset)', updated_at=?
		WHERE status IN ('pending','transforming') AND updated_at != '' AND updated_at < ?`,
		nowUTC(), cutoff)
	if err != nil {
		return 0, fmt.Errorf("reup: sweep transform kẹt: %w", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// SweepStuckDownloading (Đợt Y): video kẹt ở "downloading" quá maxAge
// (crash/kill giữa SetStatus và tải xong) → trả về "queued" để tải lại,
// xoá file dở nếu còn. Trả về số video đã reset.
func (s *Store) SweepStuckDownloading(maxAge time.Duration) (int, error) {
	cutoff := time.Now().Add(-maxAge).UTC().Format(time.RFC3339)
	rows, err := s.db.Query(`SELECT id, file_path FROM reup_videos
		WHERE status='downloading' AND updated_at != '' AND updated_at < ?`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("reup: sweep downloading kẹt: %w", err)
	}
	var ids []int64
	var paths []string
	for rows.Next() {
		var id int64
		var fp string
		if err := rows.Scan(&id, &fp); err != nil {
			rows.Close()
			return 0, fmt.Errorf("reup: sweep downloading kẹt: %w", err)
		}
		ids = append(ids, id)
		paths = append(paths, fp)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("reup: sweep downloading kẹt: %w", err)
	}
	n := 0
	for i, id := range ids {
		if paths[i] != "" {
			_ = os.Remove(paths[i]) // file dở — lỗi thì kệ, tải lại sẽ ghi đè
		}
		if _, uerr := s.db.Exec(`UPDATE reup_videos SET status='queued',
			file_path='', fail_reason='', updated_at=? WHERE id=?`, nowUTC(), id); uerr != nil {
			return n, fmt.Errorf("reup: sweep downloading kẹt: %w", uerr)
		}
		n++
	}
	return n, nil
}

// SetPostTransformed ghi nhận transform xong.
func (s *Store) SetPostTransformed(id int64, filePath string) error {
	_, err := s.db.Exec(`UPDATE reup_posts SET status=?, file_path=?, fail_reason='' WHERE id=?`,
		PostTransformed, filePath, id)
	if err != nil {
		return fmt.Errorf("reup: ghi nhận transform xong: %w", err)
	}
	return nil
}

// SetPostTarget gắn kênh + nền tảng cho bài.
func (s *Store) SetPostTarget(id int64, accountRef, platform string) error {
	_, err := s.db.Exec(`UPDATE reup_posts SET account_ref=?, platform=? WHERE id=?`,
		accountRef, platform, id)
	if err != nil {
		return fmt.Errorf("reup: gắn kênh cho bài: %w", err)
	}
	return nil
}

// MarkPostPosted ghi nhận đã đăng (+ id/url trên nền tảng).
func (s *Store) MarkPostPosted(id int64, platform, remoteID, remoteURL string) error {
	_, err := s.db.Exec(`UPDATE reup_posts SET status=?, platform=?,
		remote_id=?, remote_url=?, posted_at=? WHERE id=?`,
		PostPosted, platform, remoteID, remoteURL, nowUTC(), id)
	if err != nil {
		return fmt.Errorf("reup: ghi nhận đã đăng: %w", err)
	}
	return nil
}

// SetPostMetrics cập nhật view thật của bài (từ API nền tảng).
func (s *Store) SetPostMetrics(id int64, views int64) error {
	if views < 0 {
		return fmt.Errorf("reup: view không hợp lệ (%d)", views)
	}
	_, err := s.db.Exec(`UPDATE reup_posts SET views=?, metrics_at=? WHERE id=?`,
		views, nowUTC(), id)
	if err != nil {
		return fmt.Errorf("reup: cập nhật metrics: %w", err)
	}
	return nil
}

// CountPostedSince đếm bài đã đăng của kênh từ mốc RFC3339 (giới hạn/ngày).
func (s *Store) CountPostedSince(accountRef, since string) (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM reup_posts
		WHERE status=? AND account_ref=? AND posted_at >= ?`,
		PostPosted, accountRef, since).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("reup: đếm bài đã đăng: %w", err)
	}
	return n, nil
}

// VideosNeedingTransform liệt kê video đã tải xong nhưng chưa có bài nào
// ở trạng thái còn hiệu lực (pending/transforming/transformed/posting/
// posted). Bài failed được phép làm lại.
func (s *Store) VideosNeedingTransform(limit int) ([]Video, error) {
	q := `SELECT id, source_id, douyin_id, url, file_path, sha256,
		duration_sec, resolution, play_count, author, title, watermark_free, via,
		status, fail_reason, qc_method, downloaded_at, created_at
		FROM reup_videos v WHERE v.status='downloaded'
		AND NOT EXISTS (
			SELECT 1 FROM reup_posts p
			WHERE (',' || p.video_ids || ',') LIKE '%,' || CAST(v.id AS TEXT) || ',%'
			AND p.status != 'failed')
		ORDER BY v.id ASC`
	if limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", limit)
	}
	rows, err := s.db.Query(q)
	if err != nil {
		return nil, fmt.Errorf("reup: video chờ transform: %w", err)
	}
	defer rows.Close()
	var out []Video
	for rows.Next() {
		var v Video
		if err := rows.Scan(&v.ID, &v.SourceID, &v.DouyinID, &v.URL, &v.FilePath,
			&v.SHA256, &v.DurationSec, &v.Resolution, &v.PlayCount, &v.Author,
			&v.Title, &v.WatermarkFree, &v.Via, &v.Status, &v.FailReason,
			&v.QCMethod, &v.DownloadedAt, &v.CreatedAt); err != nil {
			return nil, fmt.Errorf("reup: video chờ transform: %w", err)
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// VideosForCompilation lấy n video đã tải cùng nguồn (mới nhất) cho mức 2.
// Loại video đã có bài còn hiệu lực (khác failed) — cùng mẫu NOT EXISTS
// với VideosNeedingTransform, chặn tick 15 phút tạo compilation trùng
// từ cùng 3 video (Đợt Y).
func (s *Store) VideosForCompilation(sourceID int64, n int) ([]Video, error) {
	rows, err := s.db.Query(`SELECT id, source_id, douyin_id, url, file_path, sha256,
		duration_sec, resolution, play_count, author, title, watermark_free, via,
		status, fail_reason, qc_method, downloaded_at, created_at
		FROM reup_videos v WHERE v.status='downloaded' AND v.source_id=?
		AND NOT EXISTS (
			SELECT 1 FROM reup_posts p
			WHERE (',' || p.video_ids || ',') LIKE '%,' || CAST(v.id AS TEXT) || ',%'
			AND p.status != 'failed')
		ORDER BY play_count DESC, id DESC LIMIT ?`, sourceID, n)
	if err != nil {
		return nil, fmt.Errorf("reup: video compilation: %w", err)
	}
	defer rows.Close()
	var out []Video
	for rows.Next() {
		var v Video
		if err := rows.Scan(&v.ID, &v.SourceID, &v.DouyinID, &v.URL, &v.FilePath,
			&v.SHA256, &v.DurationSec, &v.Resolution, &v.PlayCount, &v.Author,
			&v.Title, &v.WatermarkFree, &v.Via, &v.Status, &v.FailReason,
			&v.QCMethod, &v.DownloadedAt, &v.CreatedAt); err != nil {
			return nil, fmt.Errorf("reup: video compilation: %w", err)
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
