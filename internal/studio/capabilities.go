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
	CapVideoGen: "Quay video (Veo)",
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

// GetCapabilities trả về cả hai khả năng theo thứ tự cố định; chưa từng
// kiểm tra → "unknown".
func (s *Studio) GetCapabilities() []AICapability {
	out := []AICapability{
		{Key: CapImageGen, Label: capabilityLabels[CapImageGen], Status: CapUnknown},
		{Key: CapVideoGen, Label: capabilityLabels[CapVideoGen], Status: CapUnknown},
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

// VideoCapOK báo key có quay được video không (theo lần kiểm tra gần nhất).
func (s *Studio) VideoCapOK() bool {
	for _, c := range s.GetCapabilities() {
		if c.Key == CapVideoGen {
			return c.Status == CapOK
		}
	}
	return false
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

// RefreshVideoStatusFromHistory suy trạng thái quay video từ N lần quay
// thật gần nhất (method từng shot — Wave 1 đã log): có shot nào "veo"
// thành công → ok; toàn "anh-tts" → fail (key không tạo được video);
// chưa quay lần nào → unknown. Chỉ ghi khi trạng thái hiện tại là
// unknown — probe tay luôn thắng.
func (s *Studio) RefreshVideoStatusFromHistory() {
	for _, c := range s.GetCapabilities() {
		if c.Key == CapVideoGen && c.Status != CapUnknown {
			return
		}
	}
	rows, err := s.db.Query(
		`SELECT method FROM studio_assets
		 WHERE kind='shot' AND method != '' ORDER BY id DESC LIMIT 30`)
	if err != nil {
		return
	}
	defer rows.Close()
	n := 0
	veoOK := false
	for rows.Next() {
		var m string
		if err := rows.Scan(&m); err != nil {
			continue
		}
		n++
		if m == "veo" {
			veoOK = true
		}
	}
	switch {
	case veoOK:
		_ = s.SetCapability(CapVideoGen, CapOK, "shot gần nhất quay bằng Veo thành công")
	case n > 0:
		_ = s.SetCapability(CapVideoGen, CapFail,
			fmt.Sprintf("%d lần quay gần nhất đều rớt về ảnh + giọng đọc — key có thể không tạo được video", n))
	default:
		_ = s.SetCapability(CapVideoGen, CapUnknown, "chưa có lần quay nào")
	}
}

// ProbeVideoGen quay thử đúng 1 clip 8s bằng Veo — TỐN TIỀN THẬT (~8s
// billed). Chỉ gọi từ nút bấm tay "Kiểm tra quay video" (đã cảnh báo chi
// phí trên UI), không bao giờ tự chạy ngầm.
func (s *Studio) ProbeVideoGen(ctx context.Context) error {
	if s.mg == nil || !s.mg.Healthy(ctx) {
		_ = s.SetCapability(CapVideoGen, CapFail, "thiếu GEMINI_API_KEYS hoặc key lỗi")
		return fmt.Errorf("media-gen chưa sẵn sàng")
	}
	img := filepath.Join(os.TempDir(), "aicos-probe-key.png")
	defer os.Remove(img)
	if err := s.mg.GenerateImage(ctx,
		"a red sports car on an empty road at sunset, cinematic photo, no text",
		nil, img); err != nil {
		_ = s.SetCapability(CapVideoGen, CapFail, "không vẽ được keyframe kiểm tra: "+truncErr(err, 120))
		return fmt.Errorf("keyframe kiểm tra: %w", err)
	}
	out := filepath.Join(os.TempDir(), "aicos-probe-veo.mp4")
	defer os.Remove(out)
	err := s.mg.GenerateVideo(ctx,
		"the red sports car drives slowly forward on the road, cinematic, natural motion, no text",
		img, veoMaxSeconds, "16:9", out)
	if err != nil {
		_ = s.SetCapability(CapVideoGen, CapFail, "Veo lỗi: "+truncErr(err, 160))
		return err
	}
	_ = s.SetCapability(CapVideoGen, CapOK, "quay thử 8s thành công")
	return nil
}

func truncErr(err error, n int) string {
	s := strings.ReplaceAll(err.Error(), "\n", " ")
	if len([]rune(s)) > n {
		s = string([]rune(s)[:n]) + "…"
	}
	return s
}
