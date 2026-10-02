package reup

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite" // pure-Go sqlite driver
)

// Store lưu nguồn + video reup (SQLite WAL, một DB một nhiệm vụ: reup.db).
type Store struct {
	db *sql.DB
}

// schemaVersion hiện tại; migration qua PRAGMA user_version.
// v1 (Đợt D): reup_sources + reup_videos + reup_posts (posts để Đợt E dùng).
const schemaVersion = 1

// NewStore mở (hoặc tạo) reup.db trong dataDir.
func NewStore(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("reup: open db: %w", err)
	}
	db.SetMaxOpenConns(1)
	for _, p := range []string{"PRAGMA journal_mode=WAL", "PRAGMA busy_timeout=5000"} {
		if _, err := db.Exec(p); err != nil {
			db.Close()
			return nil, fmt.Errorf("reup: pragma %q: %w", p, err)
		}
	}
	var cur int
	if err := db.QueryRow("PRAGMA user_version").Scan(&cur); err != nil {
		db.Close()
		return nil, fmt.Errorf("reup: pragma user_version: %w", err)
	}
	if cur < schemaVersion {
		if err := migrate(db); err != nil {
			db.Close()
			return nil, err
		}
		if _, err := db.Exec(fmt.Sprintf("PRAGMA user_version = %d", schemaVersion)); err != nil {
			db.Close()
			return nil, fmt.Errorf("reup: stamp user_version: %w", err)
		}
	}
	return &Store{db: db}, nil
}

func migrate(db *sql.DB) error {
	_, err := db.Exec(`
	CREATE TABLE IF NOT EXISTS reup_sources(
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		kind TEXT NOT NULL,            -- user | hashtag
		value TEXT NOT NULL,           -- unique_id Douyin hoặc tên hashtag
		display_name TEXT NOT NULL DEFAULT '',
		enabled INTEGER NOT NULL DEFAULT 1,
		added_at TEXT NOT NULL,
		UNIQUE(kind, value));
	CREATE TABLE IF NOT EXISTS reup_videos(
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		source_id INTEGER NOT NULL DEFAULT 0,
		douyin_id TEXT NOT NULL DEFAULT '',
		url TEXT NOT NULL DEFAULT '',
		file_path TEXT NOT NULL DEFAULT '',
		sha256 TEXT NOT NULL DEFAULT '',
		duration_sec REAL NOT NULL DEFAULT 0,
		resolution TEXT NOT NULL DEFAULT '',
		play_count INTEGER NOT NULL DEFAULT 0,
		author TEXT NOT NULL DEFAULT '',
		title TEXT NOT NULL DEFAULT '',
		watermark_free INTEGER NOT NULL DEFAULT 0,
		via TEXT NOT NULL DEFAULT '',  -- ytdlp | tikwm
		status TEXT NOT NULL DEFAULT 'queued', -- queued|downloading|downloaded|failed
		fail_reason TEXT NOT NULL DEFAULT '',
		qc_method TEXT NOT NULL DEFAULT '',
		downloaded_at TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL);
	CREATE INDEX IF NOT EXISTS idx_reup_videos_douyin ON reup_videos(douyin_id);
	CREATE INDEX IF NOT EXISTS idx_reup_videos_sha ON reup_videos(sha256);
	CREATE INDEX IF NOT EXISTS idx_reup_videos_status ON reup_videos(status);
	-- Đợt E dùng: mỗi video → transform → bài đăng per kênh/nền tảng.
	CREATE TABLE IF NOT EXISTS reup_posts(
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		video_id INTEGER NOT NULL,
		account_ref TEXT NOT NULL DEFAULT '',
		platform TEXT NOT NULL DEFAULT '',
		transform_level INTEGER NOT NULL DEFAULT 0,
		status TEXT NOT NULL DEFAULT 'pending',
		posted_at TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL,
		FOREIGN KEY(video_id) REFERENCES reup_videos(id));`)
	if err != nil {
		return fmt.Errorf("reup: migrate: %w", err)
	}
	return nil
}

// Close đóng DB.
func (s *Store) Close() error { return s.db.Close() }

func nowUTC() string { return time.Now().UTC().Format(time.RFC3339) }

// ---------------------------------------------------------------- sources

// Source là một nguồn Douyin (user hoặc hashtag).
type Source struct {
	ID          int64
	Kind        string // "user" | "hashtag"
	Value       string
	DisplayName string
	Enabled     bool
	AddedAt     string
}

// KindLabel nhãn tiếng Việt.
func (s Source) KindLabel() string {
	if s.Kind == "hashtag" {
		return "hashtag"
	}
	return "user"
}

// AddSource thêm nguồn; trùng (kind, value) thì trả về nguồn cũ (không lỗi).
func (s *Store) AddSource(kind, value, displayName string) (Source, error) {
	kind = normKind(kind)
	value = strings.TrimPrefix(strings.TrimPrefix(trimWS(value), "#"), "@")
	if value == "" {
		return Source{}, fmt.Errorf("reup: giá trị nguồn trống")
	}
	if displayName == "" {
		displayName = value
	}
	_, err := s.db.Exec(`INSERT INTO reup_sources(kind, value, display_name, enabled, added_at)
		VALUES (?,?,?,?,?)
		ON CONFLICT(kind, value) DO UPDATE SET display_name=excluded.display_name`,
		kind, value, displayName, 1, nowUTC())
	if err != nil {
		return Source{}, fmt.Errorf("reup: thêm nguồn: %w", err)
	}
	return s.GetSource(kind, value)
}

// GetSource đọc một nguồn.
func (s *Store) GetSource(kind, value string) (Source, error) {
	var src Source
	var enabled int
	err := s.db.QueryRow(`SELECT id, kind, value, display_name, enabled, added_at
		FROM reup_sources WHERE kind=? AND value=?`, normKind(kind), trimWS(value)).
		Scan(&src.ID, &src.Kind, &src.Value, &src.DisplayName, &enabled, &src.AddedAt)
	if err != nil {
		return Source{}, fmt.Errorf("reup: đọc nguồn: %w", err)
	}
	src.Enabled = enabled == 1
	return src, nil
}

// ListSources liệt kê nguồn (mới nhất trước).
func (s *Store) ListSources() ([]Source, error) {
	rows, err := s.db.Query(`SELECT id, kind, value, display_name, enabled, added_at
		FROM reup_sources ORDER BY id DESC`)
	if err != nil {
		return nil, fmt.Errorf("reup: liệt kê nguồn: %w", err)
	}
	defer rows.Close()
	var out []Source
	for rows.Next() {
		var src Source
		var enabled int
		if err := rows.Scan(&src.ID, &src.Kind, &src.Value, &src.DisplayName, &enabled, &src.AddedAt); err != nil {
			return nil, fmt.Errorf("reup: liệt kê nguồn: %w", err)
		}
		src.Enabled = enabled == 1
		out = append(out, src)
	}
	return out, rows.Err()
}

// EnabledSources liệt kê nguồn đang bật.
func (s *Store) EnabledSources() ([]Source, error) {
	all, err := s.ListSources()
	if err != nil {
		return nil, err
	}
	var out []Source
	for _, src := range all {
		if src.Enabled {
			out = append(out, src)
		}
	}
	return out, nil
}

// SetSourceEnabled bật/tắt nguồn.
func (s *Store) SetSourceEnabled(id int64, enabled bool) error {
	v := 0
	if enabled {
		v = 1
	}
	res, err := s.db.Exec(`UPDATE reup_sources SET enabled=? WHERE id=?`, v, id)
	if err != nil {
		return fmt.Errorf("reup: bật/tắt nguồn: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("reup: không tìm thấy nguồn #%d", id)
	}
	return nil
}

// DeleteSource xoá nguồn (video đã tải giữ lại).
func (s *Store) DeleteSource(id int64) error {
	res, err := s.db.Exec(`DELETE FROM reup_sources WHERE id=?`, id)
	if err != nil {
		return fmt.Errorf("reup: xoá nguồn: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("reup: không tìm thấy nguồn #%d", id)
	}
	return nil
}

// ----------------------------------------------------------------- videos

// Trạng thái video.
const (
	StatusQueued      = "queued"
	StatusDownloading = "downloading"
	StatusDownloaded  = "downloaded"
	StatusFailed      = "failed"
)

// Video là một video Douyin trong kho reup.
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
func (v Video) WatermarkLabel() string {
	if v.Status != StatusDownloaded {
		return "—"
	}
	if v.Via == "tikwm" || v.WatermarkFree {
		return "không watermark"
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
