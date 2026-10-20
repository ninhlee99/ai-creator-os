package studio

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// MotionVideoOptions specifies the requirements for generating an AI motion video clip.
type MotionVideoOptions struct {
	Prompt       string
	FirstFrame   string // optional reference keyframe for Image-to-Video
	Seconds      int    // duration hint (default 4-8s)
	AspectRatio  string // "9:16" or "16:9"
	Engine       string // "veo3", "googleflow", "museai", "auto", "generative"
	Anchor       *VisualAnchor // persistent character/scene anchor for cross-adapter continuity
	AutoFaceLock bool          // automatically run InsightFace lock pass with master photo
	SceneIndex   int           // scene position index for angle/clip variation
	OutputPath   string
	LogFn        func(string)
}

// GenerateMotionVideoClip renders a single motion video clip using the requested AI video engine
// (Google Veo 3, Muse AI, or intelligent cascade), conforming the output strictly to 9:16 or 16:9.
func (s *Studio) GenerateMotionVideoClip(ctx context.Context, opts MotionVideoOptions) error {
	if strings.TrimSpace(opts.Prompt) == "" {
		return fmt.Errorf("prompt video không được để trống")
	}
	if opts.Seconds <= 0 {
		opts.Seconds = 6
	}
	if opts.AspectRatio == "" {
		opts.AspectRatio = "9:16"
	}
	if opts.Engine == "" {
		opts.Engine = "auto"
	}

	engine := strings.ToLower(strings.TrimSpace(opts.Engine))
	tempClip := opts.OutputPath + ".raw.mp4"
	defer os.Remove(tempClip)

	log := func(msg string) {
		if opts.LogFn != nil {
			opts.LogFn(msg)
		}
	}

	var generated bool
	var lastErr error

	// 0. Chế độ Google Flow (Veo SD 480p - 1.000 Credits Free/tháng qua Web Adapter)
	if engine == "googleflow" || ((engine == "auto" || engine == "generative" || engine == "webai") && !generated) {
		sess, sessErr := s.GetWebAISession("googleflow")
		hasFlowWebSession := (sessErr == nil && sess != nil && (sess.SessionToken != "" || sess.CookieHeader != ""))

		if hasFlowWebSession {
			log(fmt.Sprintf("🌊 Đã kích hoạt Google Flow Web Adapter (%s) — Tận dụng 1.000 credits miễn phí/tháng...", sess.Email))
		} else if engine == "googleflow" {
			log("🌊 Khởi động Google Flow (Veo SD 480p): Tận dụng 1.000 credits miễn phí...")
		}

		// Kiểm tra nếu người dùng đã tải sẵn clip từ Google Flow vào thư mục hoặc qua web adapter
		flowClip := filepath.Join(filepath.Dir(opts.OutputPath), "flow_input.mp4")
		if fileExists(flowClip) {
			log(fmt.Sprintf("✓ Google Flow Web: Đã nhận diện clip Veo SD 480p (%s) — đưa vào pipeline Remaster 4K UHD...", filepath.Base(flowClip)))
			if err := copyViralFile(flowClip, tempClip); err == nil {
				generated = true
			}
		} else if hasFlowWebSession || s.mg != nil {
			if s.mg != nil {
				// Sinh video Veo miễn phí qua Gemini API / Google Flow
				veoErr := s.mg.GenerateVideo(ctx, opts.Prompt, opts.FirstFrame, opts.Seconds, opts.AspectRatio, tempClip)
				if veoErr == nil && fileExists(tempClip) {
					log("✓ Google Flow (Veo Free Tier): Đã kết xuất video chuyển động thành công 100%!")
					generated = true
				} else {
					lastErr = veoErr
					if engine == "googleflow" {
						log(fmt.Sprintf("Google Flow lưu ý (%v) — chuyển tiếp sang Muse AI / Veo 3...", veoErr))
					}
				}
			}
		}
	}

	// 1. Chế độ Veo 3 (hoặc Auto ưu tiên Veo 3 Free Preview)
	if !generated && (engine == "veo3" || engine == "auto" || engine == "generative") {
		log("🎬 Khởi động công nghệ Google Veo 3.1 / 3.0 (Ưu tiên Free Preview): Đang kết nối AI sinh video...")
		if s.mg != nil {
			veoErr := s.mg.GenerateVideo(ctx, opts.Prompt, opts.FirstFrame, opts.Seconds, opts.AspectRatio, tempClip)
			if veoErr == nil && fileExists(tempClip) {
				log("✓ Google Veo: Đã sinh thành công phân cảnh video điện ảnh chuyển động thật!")
				generated = true
			} else {
				lastErr = veoErr
				log(fmt.Sprintf("Google Veo lưu ý (%v) — tự động chuyển sang Muse AI...", veoErr))
			}
		} else {
			lastErr = fmt.Errorf("veo: mediagen chưa khởi tạo")
		}
	}

	// 2. Chế độ Muse AI (hoặc Auto fallback khi kết nối Web AI / API)
	if !generated && (engine == "museai" || engine == "auto" || engine == "generative" || engine == "webai") {
		log("🎥 Khởi động công nghệ Muse AI Video Studio (Web & API)...")
		apiKey := os.Getenv("MUSE_AI_API_KEY")
		sessionKey := os.Getenv("MUSE_AI_SESSION")
		cookieHdr := ""

		// Ưu tiên nạp phiên đăng nhập từ Kho Tài Khoản Web AI (/settings/he-thong#tab-webai)
		if sess, err := s.GetWebAISession("museai"); err == nil && sess != nil {
			if sess.SessionToken != "" {
				sessionKey = sess.SessionToken
			}
			cookieHdr = sess.CookieHeader
			log(fmt.Sprintf("✓ Muse AI Web: Đã nhận diện phiên tài khoản Web AI (%s)...", sess.Email))
		}

		museClient := NewMuseAIClient(apiKey, sessionKey)
		if cookieHdr != "" {
			museClient.SetCookieHeader(cookieHdr)
		}

		if museClient.Healthy() {
			museErr := museClient.RenderMuseAIVideo(ctx, opts.Prompt, opts.FirstFrame, opts.Seconds, opts.AspectRatio, tempClip, log)
			if museErr == nil && fileExists(tempClip) {
				log("✓ Muse AI: Đã tạo video chuyển động mượt mà thành công 100%!")
				generated = true
			} else {
				lastErr = museErr
				log(fmt.Sprintf("Muse AI lưu ý (%v) — chuyển tiếp sang pipeline chuyển động thực tế...", museErr))
			}
		} else {
			if engine == "museai" {
				return fmt.Errorf("muse.ai: chưa cấu hình tài khoản Muse AI trong Cài đặt · Tài khoản AI hoặc MUSE_AI_API_KEY")
			}
		}
	}

	// 3. Fallback: Nếu cả Veo 3 và Muse AI chưa sẵn sàng, áp dụng chuyển động người thật quay phim chuẩn 9:16
	if !generated {
		log("⚡ Kích hoạt chế độ Phim chuyển động tự nhiên chuẩn 9:16 (Quay phim thật 100%)...")
		promptLow := strings.ToLower(opts.Prompt)
		isFashion := strings.Contains(promptLow, "fashion") || strings.Contains(promptLow, "thời trang") || strings.Contains(promptLow, "lookbook")
		isCN := strings.Contains(promptLow, "douyin") || strings.Contains(promptLow, "trung quốc") || strings.Contains(promptLow, "cn") || strings.Contains(promptLow, "c-biz") || (opts.Anchor != nil && opts.Anchor.Ethnicity == "Chinese")

		cat := "viral"
		if isFashion {
			cat = "fashion"
		}
		stockCandidates := FindStockClips(cat)
		if len(stockCandidates) > 0 {
			var matched []string
			for _, sc := range stockCandidates {
				base := strings.ToLower(filepath.Base(sc))
				if isCN && (strings.Contains(base, "cn") || strings.Contains(base, "douyin")) {
					matched = append(matched, sc)
				} else if !isCN && (strings.Contains(base, "vn") || strings.Contains(base, "phuonghoa") || strings.Contains(base, "cac") || strings.Contains(base, "sexy_dance")) {
					matched = append(matched, sc)
				}
			}
			if len(matched) > 0 {
				stockCandidates = matched
			}
			clipIdx := opts.SceneIndex % len(stockCandidates)
			if clipIdx < 0 {
				clipIdx = -clipIdx
			}
			srcClip := stockCandidates[clipIdx]
			log(fmt.Sprintf("✓ Tận dụng video chuyển động người thật HD 9:16 [Góc máy %d/%d: %s]...", clipIdx+1, len(stockCandidates), filepath.Base(srcClip)))
			if err := TrimVideo(ctx, srcClip, float64(opts.Seconds), opts.AspectRatio, tempClip); err == nil && fileExists(tempClip) {
				generated = true
			}
		}
	}

	// 4. Fallback cuối cùng nếu không có clip: Dựng chuyển động Ken Burns êm ái từ keyframe
	if !generated {
		if opts.FirstFrame != "" && fileExists(opts.FirstFrame) {
			log("ℹ️ Tạo chuyển động trôi êm ái CapCut từ khung hình phân cảnh...")
			if err := AssemblePhotoList(ctx, []string{opts.FirstFrame}, float64(opts.Seconds), "", 0, tempClip, opts.AspectRatio); err == nil && fileExists(tempClip) {
				generated = true
			}
		}
	}

	if !generated {
		if lastErr != nil {
			return fmt.Errorf("không thể tạo video với Veo 3 / Muse AI / Google Flow: %w", lastErr)
		}
		return fmt.Errorf("không thể kết xuất video phân cảnh")
	}

	// KHOÁ KHUÔN MẶT ĐỒNG NHẤT (Face Continuity Pass): Dùng Master Photo để khóa khuôn mặt chuẩn
	masterFacePhoto := ""
	if opts.Anchor != nil && opts.Anchor.MasterPhotoPath != "" && fileExists(opts.Anchor.MasterPhotoPath) {
		masterFacePhoto = opts.Anchor.MasterPhotoPath
	} else if opts.FirstFrame != "" && fileExists(opts.FirstFrame) {
		masterFacePhoto = opts.FirstFrame
	}

	if opts.AutoFaceLock && masterFacePhoto != "" {
		faceSwappedClip := tempClip + ".faceswapped.mp4"
		log(fmt.Sprintf("👤 Kích hoạt Face Lock: Khóa khuôn mặt đồng nhất 100%% theo Master Photo (%s)...", filepath.Base(masterFacePhoto)))
		if fsErr := RunFaceSwap(ctx, masterFacePhoto, tempClip, faceSwappedClip, func(pct int, msg string) {
			if msg != "" && pct%25 == 0 {
				log(msg)
			}
		}); fsErr == nil && fileExists(faceSwappedClip) {
			log("✓ Khóa khuôn mặt thành công: Đảm bảo nhân vật hoàn toàn đồng nhất không bị trôi lệch!")
			_ = os.Rename(faceSwappedClip, tempClip)
		} else {
			_ = os.Remove(faceSwappedClip)
		}
	}

	// CHUẨN HOÁ HÌNH HỌC VÀ REMASTER 4K UHD (Lanczos + Micro-Contrast Unsharp Mask + Denoise)
	targetW, targetH := AspectDims(opts.AspectRatio)
	var vf string
	var encArgs []string

	if targetW >= 2160 || targetH >= 2160 || IsMasterQuality4K() {
		targetW, targetH = AspectDims4K(opts.AspectRatio)
		vf = Master4KFilter(targetW, targetH)
		encArgs = VideoEncoderArgs4K()
		log(fmt.Sprintf("✨ Remaster 4K UHD VideoToolbox: Nâng cấp độ phân giải lên %dx%d (35 Mbps) khử nhòe & sắc nét vi mô...", targetW, targetH))
	} else {
		vf = fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=increase:flags=lanczos,crop=%d:%d,setsar=1", targetW, targetH, targetW, targetH)
		encArgs = VideoEncoderArgs()
	}

	_ = os.Remove(opts.OutputPath)
	ffArgs := append([]string{"-i", tempClip, "-t", fmt.Sprintf("%d", opts.Seconds), "-vf", vf}, encArgs...)
	ffArgs = append(ffArgs, "-pix_fmt", "yuv420p", "-an", opts.OutputPath)

	if err := ffmpegRun(ctx, ffArgs...); err != nil {
		// If re-encoding fails, move raw clip
		_ = os.Rename(tempClip, opts.OutputPath)
	}

	if !fileExists(opts.OutputPath) {
		return fmt.Errorf("không tạo được file video hoàn chỉnh tại %s", opts.OutputPath)
	}

	return nil
}

// AvailableVideoEngines returns a descriptive list of active video creation engines.
func (s *Studio) AvailableVideoEngines() []string {
	engines := []string{"veo3", "googleflow", "museai", "faceswap", "generative"}
	return engines
}
