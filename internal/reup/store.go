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
// v2 (Đợt E): reup_posts thêm video_ids, file_path, fail_reason, views,
// metrics_at, remote_id, remote_url (transform + đăng + kill rule 0-view).
const schemaVersion = 2

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
		if err := migrate(db, cur); err != nil {
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

func migrate(db *sql.DB, from int) error {
	if from < 1 {
		if err := migrateV1(db); err != nil {
			return err
		}
	}
	if from < 2 {
		if err := migrateV2(db); err != nil {
			return err
		}
	}
	return nil
}

func migrateV1(db *sql.DB) error {
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
		 posted_at, created_at, video_ids, file_path, fail_reason,
		 views, metrics_at, remote_id, remote_url)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		legacyID, "", "", level, PostPending, "", nowUTC(),
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
	_, err := s.db.Exec(`UPDATE reup_posts SET status=?, fail_reason=? WHERE id=?`,
		status, failReason, id)
	if err != nil {
		return fmt.Errorf("reup: đổi trạng thái bài: %w", err)
	}
	return nil
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
func (s *Store) VideosForCompilation(sourceID int64, n int) ([]Video, error) {
	rows, err := s.db.Query(`SELECT id, source_id, douyin_id, url, file_path, sha256,
		duration_sec, resolution, play_count, author, title, watermark_free, via,
		status, fail_reason, qc_method, downloaded_at, created_at
		FROM reup_videos WHERE status='downloaded' AND source_id=?
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
