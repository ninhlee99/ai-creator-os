package studio

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Film Wave 3 — "Khả năng AI": key Gemini của Ninh KHÔNG tạo được video
// (Veo trả lỗi), nên hệ thống phải BIẾT mình làm được gì trước khi hứa.
// Bảng capabilities lưu trong studio.db:
//   - image_gen: probe thật lúc khởi động (1 lần vẽ thử, tốn tối thiểu).
//   - video_gen: KHÔNG tự probe (1 lần quay thử tốn ~8s Veo tiền thật) —
//     lấy từ lần quay thật gần nhất (method từng shot đã log từ Wave 1),
//     hoặc nút "Kiểm tra quay video" bấm tay (có cảnh báo chi phí).

const (
	CapImageGen = "image_gen"
	CapVideoGen = "video_gen"

	CapOK      = "ok"
	CapFail    = "fail"
	CapUnknown = "unknown"
)

// AICapability là một dòng trong bảng "Khả năng AI" (Cài đặt → Model local).
type AICapability struct {
	Key       string
	Label     string
	Status    string // ok | fail | unknown
	CheckedAt string
	Detail    string
}

var capabilityLabels = map[string]string{
	CapImageGen: "Vẽ ảnh (Gemini)",
	// CapVideoGen đã park cùng pipeline phim (film.go) — bảng "Khả năng AI"
	// giờ chỉ còn vẽ ảnh (dùng cho ảnh minh họa kể chuyện, đợt F).
}

func (s *Studio) ensureCapabilitiesTable() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS capabilities(
		key TEXT PRIMARY KEY, status TEXT NOT NULL DEFAULT 'unknown',
		checked_at TEXT NOT NULL DEFAULT '', detail TEXT NOT NULL DEFAULT '')`)
	return err
}

// SetCapability ghi trạng thái khả năng (kèm thời điểm kiểm tra).
func (s *Studio) SetCapability(key, status, detail string) error {
	if err := s.ensureCapabilitiesTable(); err != nil {
		return err
	}
	_, err := s.db.Exec(
		`INSERT INTO capabilities(key,status,checked_at,detail) VALUES(?,?,?,?)
		 ON CONFLICT(key) DO UPDATE SET status=excluded.status,
		 checked_at=excluded.checked_at, detail=excluded.detail`,
		key, status, time.Now().UTC().Format(time.RFC3339), detail)
	return err
}

// GetCapabilities trả về khả năng vẽ ảnh; chưa từng kiểm tra → "unknown".
func (s *Studio) GetCapabilities() []AICapability {
	out := []AICapability{
		{Key: CapImageGen, Label: capabilityLabels[CapImageGen], Status: CapUnknown},
	}
	if err := s.ensureCapabilitiesTable(); err != nil {
		return out
	}
	rows, err := s.db.Query(`SELECT key,status,checked_at,detail FROM capabilities`)
	if err != nil {
		return out
	}
	defer rows.Close()
	byKey := map[string]*AICapability{}
	for i := range out {
		byKey[out[i].Key] = &out[i]
	}
	for rows.Next() {
		var k, st, at, d string
		if err := rows.Scan(&k, &st, &at, &d); err != nil {
			continue
		}
		if c, ok := byKey[k]; ok {
			c.Status, c.CheckedAt, c.Detail = st, at, d
		}
	}
	return out
}

// ProbeImageGen vẽ thử 1 ảnh siêu đơn giản lúc khởi động — tốn tối thiểu,
// cho hệ thống biết image gen có sống không trước khi nhận job.
func (s *Studio) ProbeImageGen(ctx context.Context) error {
	if s.mg == nil || !s.mg.Healthy(ctx) {
		_ = s.SetCapability(CapImageGen, CapFail, "thiếu GEMINI_API_KEYS hoặc key lỗi")
		return fmt.Errorf("media-gen chưa sẵn sàng")
	}
	out := filepath.Join(os.TempDir(), "aicos-probe-image.png")
	defer os.Remove(out)
	err := s.mg.GenerateImage(ctx,
		"a single red apple on a white wooden table, simple product photo, no text",
		nil, out)
	if err != nil {
		_ = s.SetCapability(CapImageGen, CapFail, "vẽ thử lỗi: "+truncErr(err, 120))
		return err
	}
	_ = s.SetCapability(CapImageGen, CapOK, "vẽ thử thành công")
	return nil
}

func truncErr(err error, n int) string {
	s := strings.ReplaceAll(err.Error(), "\n", " ")
	if len([]rune(s)) > n {
		s = string([]rune(s)[:n]) + "…"
	}
	return s
}
