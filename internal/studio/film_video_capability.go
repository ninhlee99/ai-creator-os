//go:build parked

package studio

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

func (s *Studio) videoCapStatus() string {
	var st string
	if err := s.db.QueryRow(`SELECT status FROM capabilities WHERE key=?`,
		CapVideoGen).Scan(&st); err != nil {
		return CapUnknown
	}
	return st
}

func (s *Studio) VideoCapOK() bool {
	return s.videoCapStatus() == CapOK
}

// RefreshVideoStatusFromHistory suy trạng thái quay video từ N lần quay
// thật gần nhất (method từng shot — Wave 1 đã log): có shot nào "veo"
// thành công → ok; toàn "anh-tts" → fail (key không tạo được video);
// chưa quay lần nào → unknown. Chỉ ghi khi trạng thái hiện tại là
// unknown — probe tay luôn thắng.
func (s *Studio) RefreshVideoStatusFromHistory() {
	if s.videoCapStatus() != CapUnknown {
		return
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

// ---------------------------------------------------------------------------
// upscale video (park cùng pipeline phim — chỉ film dùng)
// ---------------------------------------------------------------------------

// upscaleVideoToDimensions masters a finished video to w×h with lanczos. This is an
// UPSCALE, not native 4K — the UI must label it honestly. Slow: only for
// finals the user explicitly asks to upscale.
func upscaleVideoToDimensions(ctx context.Context, src, dst string, w, h int) error {
	if w <= 0 || h <= 0 {
		return fmt.Errorf("upscale: kích thước %dx%d không hợp lệ", w, h)
	}
	if filepath.Clean(src) == filepath.Clean(dst) {
		return fmt.Errorf("upscale: đường dẫn nguồn và đích phải khác nhau")
	}
	tmp := dst + ".partial.mp4"
	_ = os.Remove(tmp)
	defer os.Remove(tmp)
	args := []string{"-i", src, "-map", "0:v:0", "-map", "0:a?", "-vf", VideoMasterFilter(w, h)}
	if w > 4320 || h > 4320 {
		args = append(args, VideoEncoderArgs8K()...)
	} else if w >= 2160 || h >= 2160 {
		args = append(args, VideoEncoderArgs4K()...)
	} else {
		args = append(args, videoEncoder()...)
	}
	args = append(args, "-pix_fmt", "yuv420p", "-c:a", "copy", "-movflags", "+faststart", tmp)
	if err := ffmpegRun(ctx, args...); err != nil {
		return err
	}
	if width, height := ProbeDims(ctx, tmp); width != w || height != h {
		return fmt.Errorf("upscale: đầu ra %dx%d, cần %dx%d", width, height, w, h)
	}
	if err := os.Rename(tmp, dst); err != nil {
		return err
	}
	return nil
}
