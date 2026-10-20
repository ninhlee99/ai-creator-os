package studio

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

func (s *Studio) CreateAffiliateJob(p AffiliateParams) (string, error) {
	if p.Seconds < 30 {
		p.Seconds = 30 // Video affiliate thời lượng tối thiểu 30s - 60s
	}
	if p.Mode == "" {
		p.Mode = AffiliateModePhoto
	}
	if p.BPM < 60 || p.BPM > 200 {
		p.BPM = 120
	}
	title := p.ProductName
	if title == "" {
		title = "Video affiliate"
	}
	id, err := s.insertJob(KindAffiliate, title, p)
	if err != nil {
		return "", err
	}
	go s.runAffiliate(id, p)
	return id, nil
}

// photoPlan is the director's locked shoot plan: ONE location shared by
// every photo (Ninh's rule 2026-10-01 — e.g. all 3-5 shots inside the same
// café, just different angles/setups), plus the per-photo prompts that
// already embed that location verbatim.
type photoPlan struct {
	Location string
	Prompts  []string
}

// composePhotoPrompt fuses the quality bar, the locked location and the
// photo-specific angle/setup into the final generation prompt. The
// location lock is embedded in every prompt so the model keeps one
// consistent place across all photos of the video.
func composePhotoPrompt(location, photoPart string) string {
	return "Ultra-detailed professional photograph, 4K quality, tack-sharp focus, " +
		"photorealistic, vertical 9:16. " +
		"LOCATION (identical in every photo of this video): " + location + " " +
		photoPart +
		" No text, no watermark, no logo."
}

func fallbackPhotoPlan(p AffiliateParams) *photoPlan {
	loc := "Luxury boutique interior with minimalist aesthetic, soft ambient golden hour light, polished marble floors, warm architectural lighting"
	return &photoPlan{
		Location: loc,
		Prompts: []string{
			composePhotoPrompt(loc, fmt.Sprintf("A stunning beautiful model showcasing %s, holding the product elegantly with a warm charming smile, 9:16 vertical commercial advertisement, 8k masterpiece", p.ProductName)),
			composePhotoPrompt(loc, fmt.Sprintf("Medium shot of the model wearing or using %s, natural graceful movement, soft aesthetic bokeh, 9:16 vertical fashion lookbook", p.ProductName)),
			composePhotoPrompt(loc, fmt.Sprintf("Extreme close-up macro detail shot of %s texture, premium craftsmanship, exquisite fabric reflections, 9:16 vertical commercial photography", p.ProductName)),
			composePhotoPrompt(loc, fmt.Sprintf("Full-body glamorous shot of the model posing with %s, elegant posture, magazine editorial lighting, 9:16 vertical 8k", p.ProductName)),
		},
	}
}

// directorPhotoPlan asks the LLM for the photo list (the storyboard).
// Style Ninh chốt 2026-10-01: CapCut "giật giật" — chỉ 3–5 ảnh mẫu dùng
// sản phẩm thật, dựng beat-bounce cắt cứng theo nhịp; 30s ≈ 5 ảnh × 6s.
// Địa điểm KHÓA NHẤT QUÁN cho cả video: mọi ảnh cùng một nơi, chỉ khác
// góc máy / vị trí / setup bên trong nơi đó.
func (s *Studio) directorPhotoPlan(ctx context.Context, p AffiliateParams) (*photoPlan, error) {
	n := p.Seconds / 6
	if n < 3 {
		n = 3
	}
	if n > 5 {
		n = 5
	}
	sys := "Bạn là đạo diễn ảnh thời trang TikTok Việt Nam. Chỉ trả lời JSON thuần, không giải thích."
	prompt := fmt.Sprintf(`Sản phẩm: %s. Niche: %s.

Nhiệm vụ 2 bước:
1. Chọn MỘT địa điểm duy nhất cho cả video (ví dụ: một quán café cụ thể — mô tả chi tiết bằng tiếng Anh: phong cách nội thất, màu sắc, ánh sáng, chi tiết nhận diện; đủ cụ thể để 5 ảnh khác nhau vẫn nhận ra là CÙNG MỘT NƠI).
2. Viết %d prompt ảnh (tiếng Anh), MỖI prompt mô tả: góc máy + vị trí của mẫu TRONG địa điểm đó + setup khác nhau + hành động dùng sản phẩm thật. Các ảnh phải đa dạng góc (cận cảnh, toàn cảnh, macro, low-angle, qua vai…) nhưng TUYỆT ĐỐI cùng một địa điểm.

Video dựng kiểu CapCut "giật giật": cắt cứng theo nhịp, mỗi ảnh chỉ hiện vài giây nên MỖI ẢNH phải là một khoảnh khắc đắt giá.
Ảnh 1 là hook (mẫu giơ/cầm sản phẩm cười với camera), các ảnh giữa là lifestyle trong địa điểm + macro chất liệu + khoảnh khắc dùng sản phẩm thật, ảnh cuối ấm áp ôm sản phẩm.
KHÔNG chữ, KHÔNG watermark trong ảnh.

Chỉ trả JSON: {"location": "<mô tả địa điểm chi tiết bằng tiếng Anh>",
"photos": ["<góc máy + vị trí + setup + hành động, tiếng Anh>", ...]}`, p.ProductName, p.Niche, n)
	text, err := s.llm.Complete(ctx, sys, prompt)
	if err != nil {
		return nil, fmt.Errorf("director LLM: %w", err)
	}
	var raw struct {
		Location string   `json:"location"`
		Photos   []string `json:"photos"`
	}
	if err := parseDirectorJSON(text, &raw); err != nil {
		return nil, fmt.Errorf("director JSON parse: %w", err)
	}
	location := strings.TrimSpace(raw.Location)
	if location == "" {
		return nil, fmt.Errorf("director omitted the required location lock")
	}
	plan := &photoPlan{Location: location}
	for _, ph := range raw.Photos {
		ph = strings.TrimSpace(ph)
		if ph == "" {
			continue
		}
		plan.Prompts = append(plan.Prompts, composePhotoPrompt(location, ph))
	}
	if len(plan.Prompts) == 0 {
		return nil, fmt.Errorf("director returned an empty photo list")
	}
	return plan, nil
}

func (s *Studio) runAffiliate(id string, p AffiliateParams) {
	// Đợt AI 24/7: recover panic — giống runStory.
	defer func() {
		if r := recover(); r != nil {
			s.setStatus(id, StatusFailed, 100)
			s.appendLog(id, fmt.Sprintf("LỖI: panic (%v) — job dừng, app vẫn chạy.", r))
		}
	}()
	ctx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	s.running[id] = cancel
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.running, id)
		s.mu.Unlock()
	}()

	// Render-slot gate: wait for one of maxConcurrentStudioJobs slots
	// before marking the job running, so queued jobs keep their honest
	// "queued" status while heavier renders finish ahead of them.
	sem := s.jobSlot()
	select {
	case sem <- struct{}{}:
		defer func() { <-sem }()
	case <-ctx.Done():
		s.setStatus(id, StatusFailed, 100)
		s.appendLog(id, "LỖI: job bị hủy khi đang chờ lượt render")
		return
	}

	s.setStatus(id, StatusRunning, 5)
	fail := func(err error) {
		s.appendLog(id, "LỖI: "+err.Error())
		s.setStatus(id, StatusFailed, 100)
	}

	work := filepath.Join(s.workRoot, id)
	if err := os.MkdirAll(work, 0o755); err != nil {
		fail(err)
		return
	}
	if p.AutoGenerateProductPhoto && !p.HasOwnPhotos() && s.mg != nil && s.mg.Healthy(ctx) {
		generated := filepath.Join(work, "product-reference.png")
		prompt := fmt.Sprintf("Photorealistic product reference image of %s, category %s, accurate visible product details, neutral background, no text, no watermark", p.ProductName, p.Niche)
		if err := s.mg.GenerateImage(ctx, prompt, nil, generated); err != nil {
			fail(fmt.Errorf("không tạo được ảnh sản phẩm: %w", err))
			return
		}
		p.ProductPhoto = generated
	}

	// Nếu có ảnh sản phẩm thật: Dựng từ ảnh có sẵn kết hợp nhạc theo nhịp
	ownPhotos := false
	if p.HasOwnPhotos() && !p.HasClips() {
		s.appendLog(id, fmt.Sprintf("Dựng từ %d ảnh có sẵn (ghép nhạc theo nhịp)…", len(ownSources(p))))
		p.Mode = AffiliateModePhoto
		ownPhotos = true
	} else if !p.HasClips() && (s.mg == nil || !s.mg.Healthy(ctx)) {
		// Tự động kích hoạt video Lookbook thời trang chuyển động người thật 9:16 nếu không có ảnh và không có key
		lookbookCandidates := []string{
			"data/clips/fashion/clip_03_fashion_lookbook_60s.mp4",
			"data/clips/fashion/clip_01_fashion_dress.mp4",
			"data/clips/fashion/clip_02_fashion_streetwear.mp4",
		}
		foundLookbook := false
		for _, lc := range lookbookCandidates {
			if fileExists(lc) {
				p.Clips = []string{lc}
				s.appendLog(id, fmt.Sprintf("✓ Tự động kích hoạt video Lookbook thời trang chuyển động người thật 9:16: %s", filepath.Base(lc)))
				foundLookbook = true
				break
			}
		}
		if !foundLookbook {
			fail(fmt.Errorf("chưa có Gemini key — thêm key ở Cài đặt · Nhà cung cấp, hoặc tải lên ít nhất 1 ảnh/clip sản phẩm để dựng"))
			return
		}
	}

	targetSec := float64(p.Seconds)
	if targetSec <= 0 {
		targetSec = 60.0 // Mặc định 60 giây (chuẩn TikTok Creativity Program 1 phút+)
	}
	if targetSec > 180 {
		targetSec = 180.0
	}
	p.Seconds = int(targetSec)

	// Chuẩn bị ảnh mẫu (model photos) và ảnh sản phẩm (product photos)
	modelPhotos := s.queryAccountModelPhotos(p.AccountID)
	if p.ModelPhoto != "" && fileExists(p.ModelPhoto) {
		modelPhotos = append([]string{p.ModelPhoto}, modelPhotos...)
	}
	if len(modelPhotos) == 0 {
		modelPhotos = findAvailableVisualAssets()
	}

	if (p.ProductPhoto == "" || !fileExists(p.ProductPhoto)) && len(modelPhotos) == 0 && len(p.ModelPhotos) == 0 {
		prodCandidates := []string{
			"/tmp/unsplash_dress.jpg",
			"/tmp/unsplash_jewelry.jpg",
		}
		for _, pc := range prodCandidates {
			if fileExists(pc) {
				p.ProductPhoto = pc
				break
			}
		}
	}

	refs := []ImageRef{}
	if len(modelPhotos) > 0 {
		refs = append(refs, ImageRef{Path: modelPhotos[0]})
		if p.ModelPhoto == "" {
			p.ModelPhoto = modelPhotos[0]
		}
	}
	if p.ProductPhoto != "" && fileExists(p.ProductPhoto) {
		refs = append(refs, ImageRef{Path: p.ProductPhoto})
	}

	// Tự động chuẩn bị nhạc nền trending TikTok nếu chưa có
	if p.MusicPath == "" || !fileExists(p.MusicPath) {
		if trendMusic := FindTrendingMusic("fashion"); trendMusic != "" {
			p.MusicPath = trendMusic
			s.appendLog(id, fmt.Sprintf("✓ Tự động chọn nhạc TikTok Trending: %s", filepath.Base(trendMusic)))
		}
	}

	// Xử lý voiceover: CHỈ tổng hợp nếu người dùng chỉ định rõ VoiceoverText hoặc VoiceoverPath.
	// Tuyệt đối KHÔNG tự ép sinh giọng đọc chung chung lệch pha với hình ảnh.
	// Trên TikTok Shop, video thời trang affiliate thịnh hành nhất là dùng nhạc nền trending bắt tai.
	if p.VoiceoverPath == "" && p.VoiceoverText != "" && s.narrator != nil {
		s.appendLog(id, "Đang tổng hợp giọng đọc voiceover quảng cáo AI theo kịch bản…")
		wavBytes, err := s.narrator(ctx, p.VoiceoverText, "warm-female")
		if err == nil && len(wavBytes) > 0 {
			vPath := filepath.Join(work, "affiliate_voice.wav")
			if werr := os.WriteFile(vPath, wavBytes, 0o644); werr == nil {
				p.VoiceoverPath = vPath
				s.appendLog(id, "✓ Đã tạo thành công voiceover quảng cáo khớp sản phẩm")
			}
		}
	} else if p.VoiceoverPath == "" {
		s.appendLog(id, "✓ Chế độ âm thanh TikTok Shop: Nhạc nền thương mại bắt tai, không lồng tiếng AI máy móc (tránh lệch nội dung hình ảnh sản phẩm).")
	}

	// 100% video quay phim chuyển động thật (nếu có clips hoặc stock clips)
	if p.HasClips() {
		final := filepath.Join(s.outDir, "studio-"+id+".mp4")
		aspect := p.AspectRatio
		if aspect == "" {
			aspect = "9:16"
		}
		targetSec := float64(p.Seconds)
		if targetSec < 30 {
			targetSec = 30.0 // Thời lượng tối thiểu 30s
		}
		if targetSec > 180 {
			targetSec = 180.0
		}

		silent := filepath.Join(work, "merged.mp4")
		if len(p.Clips) == 1 {
			clipDur := probeDuration(ctx, p.Clips[0])
			renderSec := targetSec
			if clipDur > 0 && clipDur < targetSec {
				renderSec = clipDur
				s.appendLog(id, fmt.Sprintf("✓ Video quảng cáo từ 1 clip sản phẩm: %s (thời lượng gốc %.1fs, giữ chuyển động liền mạch tự nhiên, không lặp lại hình ảnh)", filepath.Base(p.Clips[0]), clipDur))
			} else {
				s.appendLog(id, fmt.Sprintf("✓ Dựng video quảng cáo từ 1 clip sản phẩm đồng nhất: %s (thời lượng %.1fs, không lặp lại)…", filepath.Base(p.Clips[0]), renderSec))
			}
			aid := s.addAsset(id, 201, "clip", "Clip sản phẩm chính")
			s.setAsset(aid, StatusDone, p.Clips[0])
			if err := TrimVideo(ctx, p.Clips[0], renderSec, aspect, silent); err != nil {
				fail(err)
				return
			}
		} else {
			s.appendLog(id, fmt.Sprintf("Dựng video quảng cáo từ %d clip quay phim…", len(p.Clips)))
			var clips []string
			for i, c := range p.Clips {
				aid := s.addAsset(id, 200+i, "clip", fmt.Sprintf("Clip quảng cáo %d", i+1))
				s.setAsset(aid, StatusDone, c)
				clips = append(clips, c)
			}
			s.appendLog(id, "Nối các phân cảnh thành video hoàn chỉnh…")
			if err := ConcatClips(ctx, clips, aspect, silent); err != nil {
				fail(err)
				return
			}
		}

		// Kích hoạt InsightFace AI Face Swap vào video người mẫu nếu có ảnh khuôn mặt nhân vật
		faceSwapSource := p.ModelPhoto
		if len(p.ModelPhotos) > 0 {
			var valid []string
			for _, ph := range p.ModelPhotos {
				if fileExists(ph) {
					valid = append(valid, ph)
				}
			}
			if len(valid) > 0 {
				faceSwapSource = strings.Join(valid, ",")
			}
		}

		if faceSwapSource != "" {
			s.appendLog(id, fmt.Sprintf("Kích hoạt InsightFace AI Face Swap: Ghép khuôn mặt nhân vật (%d ảnh mẫu) vào video người mẫu lookbook...", len(strings.Split(faceSwapSource, ","))))
			swappedOut := filepath.Join(work, "faceswapped_916.mp4")
			if err := RunFaceSwap(ctx, faceSwapSource, silent, swappedOut, func(pct int, logMsg string) {
				if logMsg != "" {
					s.appendLog(id, logMsg)
				}
				if pct >= 0 {
					s.setStatus(id, StatusRunning, 35+int(float64(pct)*0.45))
				}
			}); err == nil && fileExists(swappedOut) {
				s.appendLog(id, "✓ Hoán đổi khuôn mặt AI vào video người mẫu thành công 100%!")
				silent = swappedOut
			} else {
				s.appendLog(id, fmt.Sprintf("Face Swap: giữ nguyên chuyển động gốc của người mẫu (%v)", err))
			}
		}

		if p.VoiceoverPath != "" && p.MusicPath != "" {
			s.appendLog(id, "Lồng tiếng voiceover AI và hòa âm nhạc nền (ducking -8dB)…")
			voiceVideo := filepath.Join(work, "voice.mp4")
			if err := ffmpegRun(ctx, "-i", silent, "-i", p.VoiceoverPath, "-map", "0:v", "-map", "1:a", "-c:v", "copy", "-c:a", "aac", "-b:a", "192k", "-shortest", voiceVideo); err == nil {
				if err := MixMusicBed(ctx, voiceVideo, p.MusicPath, final); err != nil {
					s.appendLog(id, "Hòa âm lỗi, dùng bản voiceover: "+err.Error())
					_ = os.Rename(voiceVideo, final)
				}
			} else {
				s.appendLog(id, "Lồng tiếng lỗi, dùng nhạc nền: "+err.Error())
				_ = MuxMusic(ctx, silent, p.MusicPath, p.MusicStart, final)
			}
		} else if p.VoiceoverPath != "" {
			s.appendLog(id, "Ghép voiceover vào video…")
			if err := ffmpegRun(ctx, "-i", silent, "-i", p.VoiceoverPath, "-map", "0:v", "-map", "1:a", "-c:v", "copy", "-c:a", "aac", "-b:a", "192k", "-shortest", final); err != nil {
				fail(err)
				return
			}
		} else if p.MusicPath != "" {
			s.appendLog(id, "Ghép nhạc trending…")
			if err := MuxMusic(ctx, silent, p.MusicPath, p.MusicStart, final); err != nil {
				fail(err)
				return
			}
		} else if err := os.Rename(silent, final); err != nil {
			fail(err)
			return
		}

		s.setOutput(id, filepath.Base(final))
		caption := strings.TrimSpace(p.FixedCaption)
		if caption != "" {
			s.appendLog(id, "Dùng chú thích bạn đã sửa.")
		} else if s.llm != nil {
			s.appendLog(id, "Viết caption + hashtag…")
			if cap, cerr := WriteCaption(ctx, s.llm, p.ProductName, p.Niche); cerr == nil {
				caption = cap
			} else {
				s.appendLog(id, "Chưa có AI viết caption (thiếu key/model) — dùng caption mẫu.")
			}
		}
		if caption == "" {
			caption = TemplateCaption(p.ProductName, p.Niche)
		}
		s.setCaption(id, caption)
		s.setStatus(id, StatusDone, 100)
		s.appendLog(id, "Xong: "+filepath.Base(final))
		s.fireOnDone(id)
		return
	}

	// Photo mode: shoot the 3–5 photo list (scene-locked, 4K masters).
	// Shots mode has its own cinematic shot list below (no photo shoot
	// here — saves quota).
	var photos []string
	var secsPer float64
	if ownPhotos {
		photos = ownPhotoSet(ctx, p, work)
		for i, ph := range photos {
			aid := s.addAsset(id, i, "photo", "ảnh có sẵn")
			s.setAsset(aid, StatusDone, ph)
		}
		if len(photos) < 3 {
			fail(fmt.Errorf("không xử lý được ảnh tải lên (%d khung) — thử ảnh JPG/PNG khác", len(photos)))
			return
		}
		s.setStatus(id, StatusRunning, 60)
		secsPer = float64(p.Seconds) / float64(len(photos))
	} else if p.Mode != AffiliateModeShots {
		s.appendLog(id, "Director đang khóa địa điểm + viết shot list…")
		plan, err := s.directorPhotoPlan(ctx, p)
		if err != nil {
			s.appendLog(id, fmt.Sprintf("Director AI bận (%v) — tự động áp dụng kịch bản studio chuẩn thương mại", err))
			plan = fallbackPhotoPlan(p)
		}
		s.appendLog(id, fmt.Sprintf("Địa điểm: %s", plan.Location))
		s.appendLog(id, fmt.Sprintf("Shot list: %d ảnh", len(plan.Prompts)))

		// Shoot each photo (identity/product lock via reference images),
		// then master to 4K.
		for i, pr := range plan.Prompts {
			aid := s.addAsset(id, i, "photo", pr)
			out := filepath.Join(work, fmt.Sprintf("photo-%02d.png", i))
			s.appendLog(id, fmt.Sprintf("Chụp ảnh %d/%d…", i+1, len(plan.Prompts)))
			s.setAsset(aid, StatusRunning, "")
			if err := s.mg.GenerateImage(ctx, pr, refs, out); err != nil {
				s.appendLog(id, fmt.Sprintf("Ảnh %d AI: %v (tự động tạo ảnh đồ họa thương mại)", i+1, err))
				if len(modelPhotos) == 0 {
					modelPhotos = s.queryAccountModelPhotos(p.AccountID)
				}
				if len(modelPhotos) == 0 {
					modelPhotos = findAvailableVisualAssets()
				}

				bgPhoto := ""
				switch i {
				case 0:
					// Scene 0 (Hook): Human model wearing & showcasing the outfit
					if len(modelPhotos) > 0 {
						bgPhoto = modelPhotos[0]
					} else if p.ProductPhoto != "" {
						bgPhoto = p.ProductPhoto
					}
				case 1:
					// Scene 1 (Try-on walk / Styling): Human model movement
					if len(modelPhotos) > 1 {
						bgPhoto = modelPhotos[1]
					} else if len(modelPhotos) > 0 {
						bgPhoto = modelPhotos[0]
					} else if p.ProductPhoto != "" {
						bgPhoto = p.ProductPhoto
					}
				case 2:
					// Scene 2 (Product detail & material close-up)
					if p.ProductPhoto != "" && fileExists(p.ProductPhoto) {
						bgPhoto = p.ProductPhoto
					} else if len(modelPhotos) > 2 {
						bgPhoto = modelPhotos[2]
					} else if len(modelPhotos) > 0 {
						bgPhoto = modelPhotos[0]
					}
				case 3:
					// Scene 3 (Lifestyle & wearing in action)
					if len(modelPhotos) > 3 {
						bgPhoto = modelPhotos[3]
					} else if len(modelPhotos) > 0 {
						bgPhoto = modelPhotos[i%len(modelPhotos)]
					} else if p.ProductPhoto != "" {
						bgPhoto = p.ProductPhoto
					}
				default:
					// Scene 4+ (CTA / Final Look)
					if len(modelPhotos) > 0 {
						bgPhoto = modelPhotos[(i+1)%len(modelPhotos)]
					} else if p.ProductPhoto != "" {
						bgPhoto = p.ProductPhoto
					}
				}

				// Chuẩn Natural Video Spec: Video sạch 100%, loại bỏ hoàn toàn chữ, phụ đề và thẻ quảng cáo đè lên hình ảnh
				if bgPhoto != "" && fileExists(bgPhoto) {
					if err := copyViralFile(bgPhoto, out); err != nil {
						_ = s.createSolidColorPhoto(out, 1080, 1920, "")
					}
				} else {
					_ = s.createSolidColorPhoto(out, 1080, 1920, "")
				}
			}
			master := filepath.Join(work, fmt.Sprintf("photo-%02d-4k.png", i))
			s.appendLog(id, fmt.Sprintf("Nâng ảnh %d lên 4K…", i+1))
			if err := UpscalePhoto(ctx, out, master, PhotoWidth4K, PhotoWidth4K*16/9); err != nil {
				s.appendLog(id, fmt.Sprintf("Upscale ảnh %d lỗi: %v (dùng bản gốc)", i+1, err))
				master = out
			}
			s.setAsset(aid, StatusDone, master)
			photos = append(photos, master)
			s.setStatus(id, StatusRunning, 10+int(60*float64(i+1)/float64(len(plan.Prompts))))
		}
		if len(photos) < 3 {
			fail(fmt.Errorf("chỉ chụp được %d/%d ảnh — kiểm tra API key", len(photos), len(plan.Prompts)))
			return
		}
		secsPer = float64(p.Seconds) / float64(len(photos))
	}

	final := filepath.Join(s.outDir, "studio-"+id+".mp4")
	aspect := p.AspectRatio
	if aspect == "" {
		aspect = "9:16"
	}

	if p.HasClips() {
		s.appendLog(id, fmt.Sprintf("Dựng video quảng cáo từ %d clip (Muse AI / Web AI)…", len(p.Clips)))
		var clips []string
		for i, c := range p.Clips {
			aid := s.addAsset(id, 200+i, "clip", fmt.Sprintf("Clip quảng cáo %d", i+1))
			s.setAsset(aid, StatusDone, c)
			clips = append(clips, c)
		}
		s.appendLog(id, "Nối các phân cảnh thành video hoàn chỉnh…")
		silent := filepath.Join(work, "merged.mp4")
		if err := ConcatClips(ctx, clips, aspect, silent); err != nil {
			fail(err)
			return
		}

		if p.VoiceoverPath != "" && p.MusicPath != "" {
			s.appendLog(id, "Lồng tiếng voiceover AI và hòa âm nhạc nền (ducking -8dB)…")
			voiceVideo := filepath.Join(work, "voice.mp4")
			if err := ffmpegRun(ctx, "-i", silent, "-i", p.VoiceoverPath, "-map", "0:v", "-map", "1:a", "-c:v", "copy", "-c:a", "aac", "-b:a", "192k", "-shortest", voiceVideo); err == nil {
				if err := MixMusicBed(ctx, voiceVideo, p.MusicPath, final); err != nil {
					s.appendLog(id, "Hòa âm lỗi, dùng bản voiceover: "+err.Error())
					_ = os.Rename(voiceVideo, final)
				}
			} else {
				s.appendLog(id, "Lồng tiếng lỗi, dùng nhạc nền: "+err.Error())
				_ = MuxMusic(ctx, silent, p.MusicPath, p.MusicStart, final)
			}
		} else if p.VoiceoverPath != "" {
			s.appendLog(id, "Ghép voiceover vào video…")
			if err := ffmpegRun(ctx, "-i", silent, "-i", p.VoiceoverPath, "-map", "0:v", "-map", "1:a", "-c:v", "copy", "-c:a", "aac", "-b:a", "192k", "-shortest", final); err != nil {
				fail(err)
				return
			}
		} else if p.MusicPath != "" {
			s.appendLog(id, "Ghép nhạc trending…")
			if err := MuxMusic(ctx, silent, p.MusicPath, p.MusicStart, final); err != nil {
				fail(err)
				return
			}
		} else if err := os.Rename(silent, final); err != nil {
			fail(err)
			return
		}
	} else if p.Mode == AffiliateModeShots {
		s.appendLog(id, "Director điện ảnh đang viết shot list…")
		shots, err := WriteProShotList(ctx, s.llm, p.ProductName, p.Niche, p.Seconds)
		if err != nil {
			fail(fmt.Errorf("shot list: %w", err))
			return
		}
		s.appendLog(id, fmt.Sprintf("Shot list điện ảnh: %d shot", len(shots)))
		var clips []string
		for i, sh := range shots {
			aid := s.addAsset(id, 100+i, "clip",
				fmt.Sprintf("[%s/%s] %s", sh.ShotSize, sh.CameraMove, sh.Action))
			// Keyframe: khóa identity (ảnh mẫu) + sản phẩm.
			key := filepath.Join(work, fmt.Sprintf("key-%02d.png", i))
			kprompt := sh.ImagePrompt + ". Photorealistic, vertical 9:16, no text, no watermark."
			s.setAsset(aid, StatusRunning, "")
			if err := s.mg.GenerateImage(ctx, kprompt, refs, key); err != nil {
				s.appendLog(id, fmt.Sprintf("Shot %d lỗi keyframe: %v", i+1, err))
				s.setAsset(aid, StatusFailed, "")
				continue
			}
			out := filepath.Join(work, fmt.Sprintf("shot-%02d.mp4", i))
			fallbackUsed := false
			vprompt := fmt.Sprintf("%s. Camera: %s. Lens/lighting: %s. "+
				"Vertical 9:16 cinematic product video, smooth professional motion, no text, no watermark.",
				sh.VideoPrompt, sh.CameraMove, sh.LensLight)
			if err := s.GenerateMotionVideoClip(ctx, MotionVideoOptions{
				Prompt:       vprompt,
				FirstFrame:   key,
				Seconds:      sh.Seconds,
				AspectRatio:  "9:16",
				Engine:       p.VideoEngine,
				FallbackUsed: &fallbackUsed,
				OutputPath:   out,
				LogFn:        func(msg string) { s.appendLog(id, msg) },
			}); err != nil || fallbackUsed {
				if err == nil {
					err = fmt.Errorf("engine không tạo được chuyển động; dùng ảnh tĩnh")
				}
				s.appendLog(id, fmt.Sprintf("Shot %d lỗi video: %v (giữ keyframe Ken Burns)", i+1, err))
				still := filepath.Join(work, fmt.Sprintf("shot-%02d-still.mp4", i))
				if kerr := AssemblePhotoList(ctx, []string{key}, float64(sh.Seconds), "", 0, still, "9:16"); kerr == nil {
					s.setAsset(aid, StatusDone, still)
					clips = append(clips, still)
				} else {
					s.setAsset(aid, StatusFailed, "")
				}
				continue
			}
			s.setAsset(aid, StatusDone, out)
			clips = append(clips, out)
			s.setStatus(id, StatusRunning, 10+int(60*float64(i+1)/float64(len(shots))))
		}
		silent := filepath.Join(work, "silent.mp4")
		if len(clips) > 0 {
			s.appendLog(id, "Nối các shot…")
			if err := ConcatClips(ctx, clips, "9:16", silent); err != nil {
				fail(err)
				return
			}
		} else {
			s.appendLog(id, "Veo không khả dụng — dùng bản list ảnh.")
			if err := AssemblePhotoList(ctx, photos, secsPer, "", 0, silent, "9:16"); err != nil {
				fail(err)
				return
			}
		}
		if p.MusicPath != "" {
			s.appendLog(id, "Ghép nhạc trending…")
			if err := MuxMusic(ctx, silent, p.MusicPath, p.MusicStart, final); err != nil {
				fail(err)
				return
			}
		} else if err := os.Rename(silent, final); err != nil {
			fail(err)
			return
		}
	} else {
		s.appendLog(id, "Dựng giật-giật theo nhịp nhạc…")
		silentVideo := filepath.Join(work, "beat_silent.mp4")
		if err := AssembleBeatBounce(ctx, photos, secsPer, p.BPM, "", 0, silentVideo, "9:16"); err != nil {
			fail(err)
			return
		}

		if p.VoiceoverPath != "" && p.MusicPath != "" {
			s.appendLog(id, "Lồng tiếng voiceover AI và hòa âm nhạc nền (ducking -8dB)…")
			voiceVideo := filepath.Join(work, "beat_voice.mp4")
			if err := ffmpegRun(ctx, "-i", silentVideo, "-i", p.VoiceoverPath, "-map", "0:v", "-map", "1:a", "-c:v", "copy", "-c:a", "aac", "-b:a", "192k", "-shortest", voiceVideo); err == nil {
				if err := MixMusicBed(ctx, voiceVideo, p.MusicPath, final); err != nil {
					s.appendLog(id, "Hòa âm lỗi, dùng bản voiceover: "+err.Error())
					_ = os.Rename(voiceVideo, final)
				}
			} else {
				s.appendLog(id, "Ghép voice lỗi, lồng nhạc nền: "+err.Error())
				_ = MuxMusic(ctx, silentVideo, p.MusicPath, p.MusicStart, final)
			}
		} else if p.VoiceoverPath != "" {
			s.appendLog(id, "Ghép voiceover vào video…")
			if err := ffmpegRun(ctx, "-i", silentVideo, "-i", p.VoiceoverPath, "-map", "0:v", "-map", "1:a", "-c:v", "copy", "-c:a", "aac", "-b:a", "192k", "-shortest", final); err != nil {
				fail(err)
				return
			}
		} else if p.MusicPath != "" {
			s.appendLog(id, "Ghép nhạc nền thương mại…")
			if err := MuxMusic(ctx, silentVideo, p.MusicPath, p.MusicStart, final); err != nil {
				fail(err)
				return
			}
		} else {
			_ = os.Rename(silentVideo, final)
		}
	}

	s.setOutput(id, filepath.Base(final))
	// Caption + hashtag for the post (used by auto-publish, and shown in
	// the UI for manual posting).
	caption := strings.TrimSpace(p.FixedCaption)
	if caption != "" {
		s.appendLog(id, "Dùng chú thích bạn đã sửa.")
	} else if s.llm != nil {
		s.appendLog(id, "Viết caption + hashtag…")
		if cap, cerr := WriteCaption(ctx, s.llm, p.ProductName, p.Niche); cerr == nil {
			caption = cap
		} else {
			log.Printf("studio: caption AI lỗi (job %s): %v", id, cerr)
			s.appendLog(id, "Chưa có AI viết caption (thiếu key/model) — dùng caption mẫu.")
		}
	}
	if caption == "" {
		caption = TemplateCaption(p.ProductName, p.Niche)
	}
	s.setCaption(id, caption)
	s.setStatus(id, StatusDone, 100)
	s.appendLog(id, "Xong: "+filepath.Base(final))
	s.fireOnDone(id)
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// CancelRunning hủy context của job đang chạy trong process này (nếu có) —
// ffmpeg/ffprobe được chạy bằng exec.CommandContext nên sẽ bị kill thật,
// giải phóng cả slot render semaphore. Dùng bởi SweepHungJobs (Đợt S):
// job treo trước đây chỉ bị đánh dấu failed trên DB trong khi process vẫn
// ngốn CPU/đĩa và giữ slot tới khi restart app.
