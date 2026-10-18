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
	if p.Seconds <= 0 {
		p.Seconds = 30
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

	// Nếu có ảnh sản phẩm thật: Ưu tiên dựng 100% bằng ảnh thật của listing kèm AdCard thương mại,
	// tránh AI hallucination vẽ sai mẫu mã, logo và quy cách sản phẩm.
	ownPhotos := false
	if p.HasOwnPhotos() {
		s.appendLog(id, fmt.Sprintf("✓ Đã nhận diện %d ảnh listing thật của sản phẩm (ảnh có sẵn) — kích hoạt dựng bằng ảnh sản phẩm thật 100%% (kèm AdCard thương mại & giá niêm yết).", len(ownSources(p))))
		p.Mode = AffiliateModePhoto
		ownPhotos = true
	} else if !p.HasClips() && (s.mg == nil || !s.mg.Healthy(ctx)) {
		fail(fmt.Errorf("chưa có Gemini key — thêm key ở Cài đặt · Nhà cung cấp, hoặc tải lên ít nhất 1 ảnh/clip sản phẩm để dựng"))
		return
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

	if p.ProductPhoto == "" || !fileExists(p.ProductPhoto) {
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
		if targetSec <= 0 {
			targetSec = 60.0 // Mặc định 60 giây (chuẩn TikTok Creativity Program 1 phút+)
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
				bgPhoto := ""
				if p.ProductPhoto != "" && (i == 0 || len(modelPhotos) == 0) {
					bgPhoto = p.ProductPhoto
				} else if len(modelPhotos) > 0 {
					bgPhoto = modelPhotos[i%len(modelPhotos)]
				} else if p.ProductPhoto != "" {
					bgPhoto = p.ProductPhoto
				}

				displayPrice := p.ProductPrice
				if displayPrice == "" {
					displayPrice = "890.000 ₫"
				}

				subtitle := "Cam kết chính hãng · Chất lượng cao cấp"
				f1 := "Sản phẩm chính hãng 100% fullbox"
				f2 := "Thiết kế hiện đại, độ hoàn thiện cao"
				f3 := "Đổi trả miễn phí 7 ngày tận nhà"
				nicheLower := strings.ToLower(p.Niche)
				if strings.Contains(nicheLower, "thời trang") || strings.Contains(nicheLower, "áo") || strings.Contains(nicheLower, "váy") {
					subtitle = "Form dáng sang trọng · Vải cao cấp"
					f1 = "Chất liệu mềm mịn, thoáng mát"
					f2 = "Tôn dáng quyến rũ, chuẩn form thiết kế"
					f3 = "Không nhăn, giữ form dáng bền đẹp"
				} else if strings.Contains(nicheLower, "mỹ phẩm") || strings.Contains(nicheLower, "da") || strings.Contains(nicheLower, "serum") {
					subtitle = "Dưỡng chất chuyên sâu · An toàn lành tính"
					f1 = "Chiết xuất thiên nhiên, kiểm nghiệm da liễu"
					f2 = "Thẩm thấu nhanh, không bết rít"
					f3 = "Phục hồi sáng khỏe sau 14 ngày"
				} else if strings.Contains(nicheLower, "công nghệ") || strings.Contains(nicheLower, "điện tử") || strings.Contains(nicheLower, "tai nghe") {
					subtitle = "Công nghệ tiên tiến · Hiệu năng vượt trội"
					f1 = "Chip xử lý thế hệ mới tốc độ cao"
					f2 = "Pin trâu, kết nối đa nền tảng tức thì"
					f3 = "Bảo hành chính hãng 12 tháng 1 đổi 1"
				} else if strings.Contains(nicheLower, "gia dụng") || strings.Contains(nicheLower, "nồi") {
					subtitle = "Tiện ích gia đình · Tiết kiệm thời gian"
					f1 = "Chất liệu cao cấp an toàn sức khỏe"
					f2 = "Công nghệ tiết kiệm điện tối ưu"
					f3 = "Dễ dàng vệ sinh, bảo hành 24 tháng"
				}

				var sceneOpts AdCardOptions
				switch i {
				case 0:
					sceneOpts = AdCardOptions{
						BgPhotoPath:   bgPhoto,
						Niche:         p.Niche,
						TopBadge:      fmt.Sprintf("✦ %s · BST MỚI 2026 ✦", strings.ToUpper(p.Niche)),
						Title:         strings.ToUpper(p.ProductName),
						Subtitle:      subtitle,
						Features:      []string{f1, f2, f3},
						PriceText:     displayPrice,
						OriginalPrice: "",
						DiscountText:  "GIẢM GIÁ ĐẶC BIỆT",
						RatingText:    "★★★★★ 4.9/5 · 2.4k+ Đã Bán",
						CTAText:       "MUA NGAY · GIỎ HÀNG GÓC TRÁI ➔",
					}
				case 1:
					sceneOpts = AdCardOptions{
						BgPhotoPath:   bgPhoto,
						Niche:         p.Niche,
						TopBadge:      "✦ ĐẶC ĐIỂM NỔI BẬT & CHẤT LIỆU ✦",
						Title:         "TỪNG CHI TIẾT TINH TẾ",
						Subtitle:      "Gia công tỉ mỉ · Tiêu chuẩn xuất khẩu",
						Features:      []string{f1, f2, f3},
						PriceText:     "CAM KẾT CHÍNH HÃNG",
						OriginalPrice: "",
						DiscountText:  "ĐỔI TRẢ 7 NGÀY",
						RatingText:    "★★★★★ 100% HÀI LÒNG",
						CTAText:       "XEM CHI TIẾT SẢN PHẨM ➔",
					}
				default:
					sceneOpts = AdCardOptions{
						BgPhotoPath:   bgPhoto,
						Niche:         p.Niche,
						TopBadge:      "✦ ƯU ĐÃI ĐỘC QUYỀN HÔM NAY ✦",
						Title:         "SỐ LƯỢNG CÒN LẠI CÓ HẠN",
						Subtitle:      "Freeship toàn quốc · Voucher hấp dẫn",
						Features:      []string{"Freeship toàn quốc đơn từ 300k", "Tặng kèm voucher ưu đãi đặc biệt", "Giao hàng hỏa tốc trong 2h"},
						PriceText:     "CHỈ CÒN 19 SUẤT",
						OriginalPrice: "",
						DiscountText:  "ƯU ĐÃI DEAL HỜI",
						RatingText:    "✦ HOT DEAL · BÁN CHẠY NHẤT ✦",
						CTAText:       "BẤM MUA NGAY TRÊN TIKTOK SHOP ➔",
					}

				}

				if verr := ComposeProductAdCard(sceneOpts, out); verr != nil {
					s.appendLog(id, fmt.Sprintf("Tạo ảnh đồ họa lỗi: %v", verr))
					s.setAsset(aid, StatusFailed, "")
					continue
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
			vprompt := fmt.Sprintf("%s. Camera: %s. Lens/lighting: %s. "+
				"Vertical 9:16 cinematic product video, smooth professional motion, no text, no watermark.",
				sh.VideoPrompt, sh.CameraMove, sh.LensLight)
			if err := s.mg.GenerateVideo(ctx, vprompt, key, sh.Seconds, "9:16", out); err != nil {
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
