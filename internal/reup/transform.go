package reup

// Transform (Đợt E): 2 mức transform cho video Douyin trước khi đăng.
//
// TRUNG THỰC — đọc trước khi sửa file này:
//   - KHÔNG kỹ thuật nào đảm bảo 100% không bị đánh bản quyền (Content ID /
//     perceptual hash ngày càng mạnh; retroactive claim có thể xảy ra).
//     Mọi comment, UI, doc của transform chỉ được ghi "giảm rủi ro",
//     KHÔNG BAO GIỜ ghi "an toàn bản quyền" / "qua Content ID".
//   - Nguy cơ lớn nhất của reup năm 2026 là video 0-view/de-boost (policy
//     "unoriginal content" của TikTok/YouTube), không phải strike.
//   - KHÔNG BAO GIỜ xuất bản video 0 transform: transform lỗi → failed,
//     không có đường nào đăng bản gốc.
//
// Mức 1 (mọi video): T5 zoom động + T3 tốc độ ±5% + T4 grade nhẹ +
// T8 voiceover bình luận tiếng Việt (TTS VieNeu local, không tốn Gemini
// key) + T7 thay audio bằng nhạc licensed + intro/outro card + caption
// tiếng Việt. Output 9:16 1080x1920.
//
// Mức 2 (video hot): Mức 1 + T9 compilation 3 clip cùng nguồn/chủ đề
// (xfade chuyển cảnh) + cấu trúc mới.

import (
	"bytes"
	"context"
	"fmt"
	"github.com/ninhlee99/ai-creator-os/internal/studio"
	"os/exec"
	"strings"
)

// Kích thước output reup: 9:16 dọc.
const (
	reupOutW = 1080
	reupOutH = 1920
	reupFPS  = 30
)

// Mức transform.
const (
	Level1 = 1 // mặc định mọi video
	Level2 = 2 // compilation 3 clip (video hot)
)

// VoiceSynth tổng hợp voiceover tiếng Việt. Hợp đồng output: WAV PCM s16le
// 24kHz mono (giống TTSProvider) — VieNeu local đi qua chain, không tốn
// Gemini key.
type VoiceSynth interface {
	Synthesize(ctx context.Context, text, voice string) ([]byte, error)
}

// Commentator viết lời bình luận ngắn từ metadata (LLM). Nil = dùng
// template trung thực (chỉ dùng sự thật có trong metadata, không bịa).
type Commentator interface {
	Complete(ctx context.Context, system, prompt string) (string, error)
}

// TransformOptions là tham số một lượt transform.
type TransformOptions struct {
	Level     int    // Level1 | Level2
	Seed      int64  // fingerprint riêng (video id + kênh) — mỗi kênh một seed
	Title     string // chữ trên intro/outro card ("" = card trơn)
	Voiceover bool   // T8: bật voiceover bình luận tiếng Việt
	Voice     string // preset voice VieNeu ("" = mặc định)
}

// Transformer chạy transform bằng FFmpeg có sẵn (pure Go gọi binary).
type Transformer struct {
	Store *Store
	Voice VoiceSynth  // nil = không tổng hợp được → fail-closed khi Voiceover=true
	LLM   Commentator // nil = template trung thực
	// MusicPath là file nhạc licensed (kho nhạc autopilot Ninh đã upload).
	// "" = không nhạc nền (chỉ voiceover).
	MusicPath string
	// WorkDir chứa file transform output.
	WorkDir string
}

// ffmpegRun chạy ffmpeg -y; lỗi kèm đuôi stderr để debug.
func ffmpegRun(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, "ffmpeg", append([]string{"-y", "-v", "error"}, args...)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		out := strings.TrimSpace(stderr.String())
		if len(out) > 400 {
			out = out[len(out)-400:]
		}
		return fmt.Errorf("ffmpeg: %v: %s", err, out)
	}
	return nil
}

// encoderArgs chọn encoder như studio (videotoolbox trên Mac, libx264
// nơi khác) — dùng chung qua studio.VideoEncoderArgs để không copy logic.
func encoderArgs() []string { return studio.VideoEncoderArgs() }

// ---------------------------------------------------------------------------
// Chuỗi filter video (mức 1 cho 1 clip) — pure function, test được không
// cần ffmpeg: cover 9:16 → zoom động (crop w/h theo t) → tốc độ ±5% theo
// seed → grade nhẹ.
// ---------------------------------------------------------------------------

// videoChainFilter dựng filter cho 1 clip. Trả về (filter, độ dài output giây).
// T5 nâng cấp (Đợt J): 4 hướng Ken Burns xoay theo seed — mỗi video một
// chuyển động khác nhau (fingerprint đa dạng hơn thay vì chỉ push-in).
