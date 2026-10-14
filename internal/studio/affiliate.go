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
		return nil, fmt.Errorf("director: %w", err)
	}
	var raw struct {
		Location string   `json:"location"`
		Photos   []string `json:"photos"`
	}
	if err := parseDirectorJSON(text, &raw); err != nil {
		return nil, err
	}
	location := strings.TrimSpace(raw.Location)
	if location == "" {
		return nil, fmt.Errorf("director: missing location lock")
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
		return nil, fmt.Errorf("director: no photos planned")
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

	// Không có key: dùng ảnh người dùng tải lên (chế độ "ảnh có sẵn").
	ownPhotos := false
	if s.mg == nil || !s.mg.Healthy(ctx) {
		if !p.HasOwnPhotos() {
			fail(fmt.Errorf("chưa có Gemini key — thêm key ở Cài đặt · Nhà cung cấp, hoặc tải lên ít nhất 1 ảnh sản phẩm để dựng bằng ảnh có sẵn"))
			return
		}
		s.appendLog(id, fmt.Sprintf("Chưa có Gemini key — dựng bằng %d ảnh bạn tải lên (chế độ ảnh có sẵn).", len(ownSources(p))))
		p.Mode = AffiliateModePhoto
		ownPhotos = true
	}

	refs := []ImageRef{}
	if p.ModelPhoto != "" {
		refs = append(refs, ImageRef{Path: p.ModelPhoto})
	}
	if p.ProductPhoto != "" {
		refs = append(refs, ImageRef{Path: p.ProductPhoto})
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
			fail(err)
			return
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
				s.appendLog(id, fmt.Sprintf("Ảnh %d lỗi: %v", i+1, err))
				s.setAsset(aid, StatusFailed, "")
				continue
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
	if p.Mode == AffiliateModeShots {
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
		if p.MusicPath != "" {
			if err := AssembleBeatBounce(ctx, photos, secsPer, p.BPM, p.MusicPath, p.MusicStart, final, "9:16"); err != nil {
				fail(err)
				return
			}
		} else {
			s.appendLog(id, "Chưa có file nhạc — xuất bản không nhạc (tải sound trending ở tab Trending rồi tạo lại).")
			if err := AssembleBeatBounce(ctx, photos, secsPer, p.BPM, "", 0, filepath.Join(work, "silent.mp4"), "9:16"); err != nil {
				fail(err)
				return
			}
			if err := os.Rename(filepath.Join(work, "silent.mp4"), final); err != nil {
				fail(err)
				return
			}
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
