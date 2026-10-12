package reup

import (
	"fmt"
	"strings"
)

type Source struct {
	ID          int64
	Kind        string // "user" | "hashtag"
	Value       string
	DisplayName string
	Enabled     bool
	AddedAt     string
	// LastError là lỗi quét gần nhất của nguồn ("" = lần quét cuối ổn).
	// UI hiện để user biết vì sao nguồn không ra video (Đợt Y).
	LastError string
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
	err := s.db.QueryRow(`SELECT id, kind, value, display_name, enabled, added_at,
		last_error FROM reup_sources WHERE kind=? AND value=?`, normKind(kind), trimWS(value)).
		Scan(&src.ID, &src.Kind, &src.Value, &src.DisplayName, &enabled, &src.AddedAt, &src.LastError)
	if err != nil {
		return Source{}, fmt.Errorf("reup: đọc nguồn: %w", err)
	}
	src.Enabled = enabled == 1
	return src, nil
}

// ListSources liệt kê nguồn (mới nhất trước).
func (s *Store) ListSources() ([]Source, error) {
	rows, err := s.db.Query(`SELECT id, kind, value, display_name, enabled, added_at,
		last_error FROM reup_sources ORDER BY id DESC`)
	if err != nil {
		return nil, fmt.Errorf("reup: liệt kê nguồn: %w", err)
	}
	defer rows.Close()
	var out []Source
	for rows.Next() {
		var src Source
		var enabled int
		if err := rows.Scan(&src.ID, &src.Kind, &src.Value, &src.DisplayName, &enabled, &src.AddedAt, &src.LastError); err != nil {
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

// SetSourceLastError ghi lỗi quét gần nhất của nguồn ("" = lần quét
// cuối ổn). UI hiện để user biết vì sao nguồn không ra video (Đợt Y).
func (s *Store) SetSourceLastError(id int64, msg string) error {
	_, err := s.db.Exec(`UPDATE reup_sources SET last_error=? WHERE id=?`, msg, id)
	if err != nil {
		return fmt.Errorf("reup: ghi lỗi quét nguồn: %w", err)
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
