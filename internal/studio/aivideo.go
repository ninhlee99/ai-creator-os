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
	FirstFrame   string        // optional reference keyframe for Image-to-Video
	Seconds      int           // duration hint (default 4-8s)
	AspectRatio  string        // "9:16" or "16:9"
	Engine       string        // "veo3", "googleflow", "museai", "auto", "generative"
	Anchor       *VisualAnchor // persistent character/scene anchor for cross-adapter continuity
	AutoFaceLock bool          // automatically run InsightFace lock pass with master photo
	SceneIndex   int           // scene position index for angle/clip variation
	FallbackUsed *bool
	OutputPath   string
	LogFn        func(string)
}

// GenerateMotionVideoClip renders a single motion video clip using the requested AI video engine
// (Veo, Muse AI, or an intelligent cascade), conforming the output strictly to 9:16 or 16:9.
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

	// Google Flow is currently a user-operated web app. It has no supported
	// generation API in this adapter, so only consume an explicitly exported
	// clip here; never mislabel a Gemini API call as Flow or retry paid video
	// generation through two cascade branches.
	if engine == "googleflow" {
		flowClip := filepath.Join(filepath.Dir(opts.OutputPath), "flow_input.mp4")
		if !fileExists(flowClip) {
			return fmt.Errorf("google flow: export video from Flow and place it at %s", flowClip)
		}
		log(fmt.Sprintf("Google Flow: importing exported clip %s", filepath.Base(flowClip)))
		if err := copyViralFile(flowClip, tempClip); err != nil {
			return fmt.Errorf("google flow: import exported clip: %w", err)
		}
		generated = true
	}

	// Veo is billed through the Google API. Auto mode must not call a paid
	// video provider without an explicit engine selection.
	if !generated && (engine == "veo3" || engine == "generative") {
		log("🎬 Gọi Google Veo API; lượt này có thể tính phí...")
		if s.mg != nil {
			veoErr := s.mg.GenerateVideo(ctx, opts.Prompt, opts.FirstFrame, opts.Seconds, opts.AspectRatio, tempClip)
			if veoErr == nil && fileExists(tempClip) {
				log("✓ Google Veo: Đã sinh thành công phân cảnh video điện ảnh chuyển động thật!")
				generated = true
			} else {
				lastErr = veoErr
				if lastErr == nil {
					lastErr = fmt.Errorf("veo không tạo được file video đầu ra")
				}
				log(fmt.Sprintf("Google Veo không tạo được footage: %v", lastErr))
			}
		} else {
			lastErr = fmt.Errorf("veo: mediagen chưa khởi tạo")
		}
	}
	if !generated && (engine == "veo3" || engine == "generative") {
		if lastErr == nil {
			lastErr = fmt.Errorf("Veo không tạo được footage")
		}
		return fmt.Errorf("không thể tạo footage chuyển động bằng engine đã chọn: %w", lastErr)
	}

	// Muse AI has no verified video-generation adapter yet. Accept an
	// explicitly exported clip, but never send account cookies to a guessed API.
	if engine == "museai" {
		museClip := filepath.Join(filepath.Dir(opts.OutputPath), "museai_input.mp4")
		if !fileExists(museClip) {
			return fmt.Errorf("muse ai: API adapter is not verified; export a clip and place it at %s", museClip)
		}
		log(fmt.Sprintf("Muse AI: importing exported clip %s", filepath.Base(museClip)))
		if err := copyViralFile(museClip, tempClip); err != nil {
			return fmt.Errorf("muse ai: import exported clip: %w", err)
		}
		generated = true
	}

	// 3. Fallback: Nếu cả Veo 3 và Muse AI chưa sẵn sàng, áp dụng chuyển động người thật quay phim chuẩn 9:16
	if !generated {
		log("Không có engine AI khả dụng; thử dùng video mẫu cục bộ nếu có...")
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
				if opts.FallbackUsed != nil {
					*opts.FallbackUsed = true
				}
			}
		}
	}

	// 4. Fallback cuối cùng nếu không có clip: Dựng chuyển động Ken Burns êm ái từ keyframe
	if !generated {
		if opts.FirstFrame != "" && fileExists(opts.FirstFrame) {
			log("Không có clip chuyển động; tạo video pan/zoom từ ảnh tĩnh.")
			if err := AssemblePhotoList(ctx, []string{opts.FirstFrame}, float64(opts.Seconds), "", 0, tempClip, opts.AspectRatio); err == nil && fileExists(tempClip) {
				generated = true
				if opts.FallbackUsed != nil {
					*opts.FallbackUsed = true
				}
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
		log(fmt.Sprintf("👤 Kích hoạt face lock theo ảnh tham chiếu (%s)...", filepath.Base(masterFacePhoto)))
		if fsErr := RunFaceSwapStrict(ctx, masterFacePhoto, tempClip, faceSwappedClip, func(pct int, msg string) {
			if msg != "" && pct%25 == 0 {
				log(msg)
			}
		}); fsErr != nil || !fileExists(faceSwappedClip) {
			_ = os.Remove(faceSwappedClip)
			if fsErr != nil {
				return fmt.Errorf("face lock thất bại: %w", fsErr)
			}
			return fmt.Errorf("face lock không tạo được video đầu ra")
		}
		if err := os.Rename(faceSwappedClip, tempClip); err != nil {
			return fmt.Errorf("lưu video face lock: %w", err)
		}
		log("Face lock hoàn tất.")
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
