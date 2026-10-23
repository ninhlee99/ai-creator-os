package studio

// ---------------------------------------------------------------------------
// Story pipeline (Đợt F — pillar 3 post-pivot): truyện ngôi thứ nhất +
// ảnh minh họa từng cảnh + giọng đọc, KHÔNG quay video.
//
//   - Viết truyện: LLM (tái dùng prompts/story.txt) → JSON {title, logline, text}.
//   - Chia cảnh: LLM → JSON {scenes:[{text, image_prompt}]} (3–12 cảnh).
//   - Ảnh minh họa: Pollinations Free API Classic — 1 ảnh / cảnh, theo aspect job.
//     Fresh multi-scene jobs require reference-image support for continuity;
//     fail before rendering when no free reference-capable provider is available.
//   - Giọng đọc: Narrator (chuỗi TTS Gemini→Edge) — 1 wav / cảnh.
//   - Dựng: tái dùng AssemblePhotoList (Ken Burns) từng cảnh → ConcatClips →
//     mux audio TTS → MixMusicBed (tùy chọn) → 16:9 MP4. KHÔNG chèn
//     chữ/phụ đề vào video (Ninh chốt 2026-10-12).
//   - QC: 1920x1080 + có audio + dài > 30s. Rớt → failed + lý do tiếng Việt.
//
// Fail-closed: thiếu LLM / thiếu ảnh / thiếu giọng đọc → không dựng video
// câm hoặc video thiếu cảnh. Không đụng code cinematic/Veo (đã parked).
// ---------------------------------------------------------------------------

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// runStory chạy pipeline kể chuyện trong goroutine nền.
func (s *Studio) runStory(id string, p StoryParams) {
	// Đợt AI 24/7: recover panic — một panic không được giết cả app
	// (pipeline chạy hàng chục phút, chạm nhiều dữ liệu ngoài).
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

	if s.mg == nil {
		fail(fmt.Errorf("media-gen chưa sẵn sàng (thiếu cấu hình tạo ảnh)"))
		return
	}
	if s.narrator == nil {
		fail(fmt.Errorf("TTS chưa sẵn sàng (không có chuỗi giọng đọc)"))
		return
	}

	work := filepath.Join(s.workRoot, id)
	if err := os.MkdirAll(work, 0o755); err != nil {
		fail(err)
		return
	}

	// Tự động chuẩn bị nhạc nền nếu bật MusicOn mà chưa chọn file cụ thể
	if p.MusicOn && (strings.TrimSpace(p.MusicPath) == "" || !fileExists(p.MusicPath)) {
		musicCat := p.Genre
		if musicCat == "" {
			musicCat = "story"
		}
		if bg := FindTrendingMusic(musicCat); bg != "" {
			p.MusicPath = bg
			s.appendLog(id, fmt.Sprintf("✓ Tự động chọn nhạc nền phù hợp (%s): %s", musicCat, filepath.Base(bg)))
		}
	}

	// 1. Use supplied manuscript or ask the writer model for one.
	if strings.TrimSpace(p.CustomText) != "" {
		s.appendLog(id, "Dùng nội dung truyện đã cung cấp…")
	} else {
		s.appendLog(id, "Nhà văn đang viết truyện…")
	}
	doc, err := s.writeStory(ctx, p)
	if err != nil {
		fail(err)
		return
	}
	title := strings.TrimSpace(doc.Title)
	if title != "" {
		s.db.Exec(`UPDATE studio_jobs SET title=? WHERE id=?`, "Kể chuyện: "+title, id)
	}
	s.appendLog(id, fmt.Sprintf("Truyện: %s (%d ký tự)", firstLine(title, doc.Text), len(doc.Text)))
	s.setStatus(id, StatusRunning, 15)

	// 2. Chia cảnh.
	s.appendLog(id, "Chia cảnh + viết prompt ảnh…")
	scenes, err := s.splitScenes(ctx, p, doc)
	if err != nil {
		fail(err)
		return
	}
	s.appendLog(id, fmt.Sprintf("Storyboard: %d cảnh", len(scenes)))
	if !supportsImageReferences(s.mg) {
		for i, scene := range scenes {
			if i > 0 && s.findDonePhoto(p.ReuseFromJobID, scene.ImagePrompt) == "" {
				fail(fmt.Errorf("kể chuyện cần ảnh tham chiếu từ cảnh đầu để giữ nhân vật đồng nhất; hiện không có provider miễn phí hỗ trợ. Dừng trước khi vẽ ảnh"))
				return
			}
		}
	}
	s.setStatus(id, StatusRunning, 25)

	// 3+4. Vẽ ảnh + đọc giọng từng cảnh (fail-closed từng cảnh).
	anchor := NewVisualAnchor(p.Topic, p.Genre, "", "")
	s.appendLog(id, fmt.Sprintf("Visual Anchor Matrix: Khóa tạo hình nhân vật (%s, %s) và bối cảnh (%s) xuyên suốt câu chuyện", anchor.ActorName, anchor.FaceTraits, anchor.Environment))

	var storyRefs []ImageRef
	var clips []string
	var wavs []string
	for i, sc := range scenes {
		aid := s.addAsset(id, i, "photo", sc.ImagePrompt)
		s.setAsset(aid, StatusRunning, "")

		img := filepath.Join(work, fmt.Sprintf("scene-%02d.png", i))
		imgPrompt := anchor.EnrichScenePrompt(sc.ImagePrompt, "slow cinematic drift", i+1, len(scenes))
		// Đợt Y: job retry tái dùng ảnh đã vẽ xong ở job cũ (cùng prompt) —
		// khỏi đốt image-gen vẽ lại.
		reused := false
		if oldImg := s.findDonePhoto(p.ReuseFromJobID, sc.ImagePrompt); oldImg != "" {
			if cerr := copyStoryFile(oldImg, img); cerr == nil {
				s.appendLog(id, fmt.Sprintf("Dùng lại ảnh cảnh %d/%d từ lần trước (khỏi vẽ lại)", i+1, len(scenes)))
				reused = true
			}
		}
		if !reused {
			s.appendLog(id, fmt.Sprintf("Vẽ ảnh cảnh %d/%d (khóa nhân vật & bối cảnh)…", i+1, len(scenes)))
			if err := s.mg.GenerateImage(ctx, imgPrompt, storyRefs, img); err != nil {
				s.setAsset(aid, StatusFailed, "")
				fail(fmt.Errorf("cảnh %d: vẽ ảnh lỗi (%v) — dừng, không dựng thiếu cảnh", i+1, err))
				return
			}
		}
		if i == 0 && fileExists(img) {
			anchor.MasterPhotoPath = img
			storyRefs = append(storyRefs, ImageRef{Path: img})
			s.appendLog(id, "✓ Đã khóa Visual Anchor Cảnh 1 làm mốc nhân vật & bối cảnh chuẩn cho toàn bộ truyện")
		}

		s.appendLog(id, fmt.Sprintf("Đọc giọng cảnh %d/%d…", i+1, len(scenes)))
		wav := filepath.Join(work, fmt.Sprintf("scene-%02d.wav", i))
		reusedWav := false
		if oldWav := s.findDoneAudio(p.ReuseFromJobID, i); oldWav != "" {
			if cerr := copyStoryFile(oldWav, wav); cerr == nil {
				s.appendLog(id, fmt.Sprintf("Dùng lại giọng đọc cảnh %d/%d từ lần trước (khỏi tổng hợp lại)", i+1, len(scenes)))
				reusedWav = true
			}
		}
		if !reusedWav {
			wavBytes, err := s.narrator(ctx, sc.Text, p.Voice)
			if err != nil {
				s.setAsset(aid, StatusFailed, "")
				fail(fmt.Errorf("cảnh %d: TTS lỗi (%v) — dừng, không tạo video câm", i+1, err))
				return
			}
			if err := os.WriteFile(wav, wavBytes, 0o644); err != nil {
				s.setAsset(aid, StatusFailed, "")
				fail(err)
			}
		}
		dur := storyProbeDuration(ctx, wav)
		if dur < 1 {
			s.setAsset(aid, StatusFailed, "")
			fail(fmt.Errorf("cảnh %d: audio giọng đọc quá ngắn (%.1fs)", i+1, dur))
			return
		}
		wavs = append(wavs, wav)

		// Clip Ken Burns cho cảnh (tái dùng AssemblePhotoList, khổ 9:16).
		clip := filepath.Join(work, fmt.Sprintf("scene-%02d.mp4", i))
		if err := AssemblePhotoList(ctx, []string{img}, dur, "", 0, clip, "9:16"); err != nil {
			s.setAsset(aid, StatusFailed, "")
			fail(fmt.Errorf("cảnh %d: dựng clip lỗi: %w", i+1, err))
			return
		}
		s.setAsset(aid, StatusDone, img)
		clips = append(clips, clip)
		s.setStatus(id, StatusRunning, 25+int(45*float64(i+1)/float64(len(scenes))))
	}

	// 5. Dựng: nối clip → mux audio TTS → nhạc nền (không chữ).
	s.appendLog(id, "Nối các cảnh…")
	videoOnly := filepath.Join(work, "video-only.mp4")
	if err := ConcatClips(ctx, clips, "9:16", videoOnly); err != nil {
		fail(fmt.Errorf("nối cảnh: %w", err))
		return
	}
	s.appendLog(id, "Ghép giọng đọc…")
	voice := filepath.Join(work, "voice.m4a")
	if err := concatWAVs(ctx, wavs, voice); err != nil {
		fail(fmt.Errorf("nối giọng đọc: %w", err))
		return
	}
	if vDur := storyProbeDuration(ctx, voice); vDur < 30 {
		fail(fmt.Errorf("QC: video chỉ dài %.0fs (cần > 30s)", vDur))
		return
	}
	final := filepath.Join(s.outDir, "studio-"+id+".mp4")
	targetSec := float64(p.DurationMinutes * 60)

	if p.DurationMinutes >= 15 && targetSec > 60 {
		s.appendLog(id, fmt.Sprintf("Mở rộng trường thiên %d phút (%.0fs) chuẩn YouTube...", p.DurationMinutes, targetSec))
		vDur := storyProbeDuration(ctx, videoOnly)
		if vDur <= 0 {
			vDur = 30
		}
		loops := int(targetSec/vDur) + 1
		longVideoOnly := filepath.Join(work, "long-video-only.mp4")
		if err := ffmpegRun(ctx, "-stream_loop", strconv.Itoa(loops), "-i", videoOnly, "-t", fmt.Sprintf("%.2f", targetSec), "-c", "copy", longVideoOnly); err != nil {
			longVideoOnly = videoOnly
		}

		if p.MusicOn && strings.TrimSpace(p.MusicPath) != "" && fileExists(p.MusicPath) {
			s.appendLog(id, "Lồng nhạc nền & soundscape trường thiên...")
			longAudio := filepath.Join(work, "long-audio.m4a")
			filter := fmt.Sprintf("[1:a]aloop=loop=-1:size=2e+09,atrim=0:%.2f,volume=0.25[bg];[0:a]apad=pad_dur=%.2f,atrim=0:%.2f[sp];[sp][bg]amix=inputs=2:duration=first:dropout_transition=0,volume=1.2,loudnorm=I=-14:TP=-1.5:LRA=11[aout]", targetSec, targetSec, targetSec)
			if err := ffmpegRun(ctx, "-i", voice, "-i", p.MusicPath, "-filter_complex", filter, "-map", "[aout]", "-c:a", "aac", "-b:a", "192k", "-t", fmt.Sprintf("%.2f", targetSec), longAudio); err == nil {
				if err := ffmpegRun(ctx, "-i", longVideoOnly, "-i", longAudio, "-c:v", "copy", "-c:a", "copy", "-t", fmt.Sprintf("%.2f", targetSec), final); err != nil {
					fail(err)
					return
				}
			} else {
				s.appendLog(id, "Nhạc nền lỗi ("+err.Error()+"), ghép audio gốc")
				longVoice := filepath.Join(work, "long-voice.m4a")
				_ = ffmpegRun(ctx, "-i", voice, "-af", fmt.Sprintf("apad=pad_dur=%.2f", targetSec), "-c:a", "aac", "-b:a", "160k", "-t", fmt.Sprintf("%.2f", targetSec), longVoice)
				if err := ffmpegRun(ctx, "-i", longVideoOnly, "-i", longVoice, "-c:v", "copy", "-c:a", "copy", "-t", fmt.Sprintf("%.2f", targetSec), final); err != nil {
					fail(err)
					return
				}
			}
		} else {
			longVoice := filepath.Join(work, "long-voice.m4a")
			_ = ffmpegRun(ctx, "-i", voice, "-af", fmt.Sprintf("apad=pad_dur=%.2f", targetSec), "-c:a", "aac", "-b:a", "160k", "-t", fmt.Sprintf("%.2f", targetSec), longVoice)
			if err := ffmpegRun(ctx, "-i", longVideoOnly, "-i", longVoice, "-c:v", "copy", "-c:a", "copy", "-t", fmt.Sprintf("%.2f", targetSec), final); err != nil {
				fail(err)
				return
			}
		}
	} else {
		withAudio := filepath.Join(work, "with-audio.mp4")
		if err := ffmpegRun(ctx, "-i", videoOnly, "-i", voice,
			"-map", "0:v", "-map", "1:a", "-c:v", "copy", "-c:a", "aac",
			"-shortest", withAudio); err != nil {
			fail(fmt.Errorf("ghép audio: %w", err))
			return
		}
		// Không chèn chữ vào video (Ninh chốt 2026-10-12): không phụ đề.
		subbed := withAudio
		if p.MusicOn && strings.TrimSpace(p.MusicPath) != "" {
			s.appendLog(id, "Lồng nhạc nền…")
			if err := MixMusicBed(ctx, subbed, p.MusicPath, final); err != nil {
				s.appendLog(id, "Nhạc nền lỗi: "+err.Error()+" (giữ bản không nhạc)")
				if err := os.Rename(subbed, final); err != nil {
					fail(err)
					return
				}
			}
		} else {
			if err := os.Rename(subbed, final); err != nil {
				fail(err)
				return
			}
		}
	}

	// 6. QC: Chuẩn 9:16 (1080x1920 hoặc 4K 2160x3840) + có audio + dài > 30s.
	s.appendLog(id, "Kiểm tra chất lượng…")
	w, h := ProbeDims(ctx, final)
	if (w != 1080 && w != 2160 && w != 1920 && w != 3840) || (h != 1920 && h != 3840 && h != 1080 && h != 2160) {
		fail(fmt.Errorf("QC: khổ video %dx%d, cần 9:16 (1080x1920 hoặc 4K 2160x3840)", w, h))
		return
	}
	if !ProbeHasAudio(ctx, final) {
		fail(fmt.Errorf("QC: video thiếu audio"))
		return
	}
	if d := storyProbeDuration(ctx, final); d < 30 {
		fail(fmt.Errorf("QC: video chỉ dài %.0fs (cần > 30s)", d))
		return
	}
	dur := storyProbeDuration(ctx, final)
	s.appendLog(id, fmt.Sprintf("QC đạt: 9:16 vertical (%dx%d), có audio, %.0fs, %d cảnh", w, h, dur, len(scenes)))

	// Master 4K Delivery: Nâng cấp phim audio trường thiên lên Master 4K UHD
	if IsMasterQuality4K() && fileExists(final) {
		s.appendLog(id, "✨ Xuất bản phim Audio trường thiên chuẩn Master 4K UHD (2160x3840 VideoToolbox 35 Mbps)...")
		story4k := filepath.Join(work, "master_story_4k.mp4")
		if err := UpscaleVideoTo4K(ctx, final, story4k, "9:16"); err == nil && fileExists(story4k) {
			_ = os.Rename(story4k, final)
			s.appendLog(id, "✓ Phim Audio đạt chuẩn điện ảnh 4K Ultra HD sắc nét tuyệt đối!")
		}
	}

	s.setOutput(id, filepath.Base(final))

	// 7. Auto-Clipper: Tự động trích xuất các clip ngắn 3-5 phút (9:16) cho TikTok/Reels nếu được bật
	if p.AutoClipShorts && dur >= 30 {
		s.appendLog(id, "Bắt đầu tự động trích xuất các clip ngắn 3–5 phút (9:16) cho TikTok & Reels…")
		clipDur := 180.0 // 3 phút mặc định
		if p.ClipDurationSec >= 30 && p.ClipDurationSec <= 300 {
			clipDur = float64(p.ClipDurationSec)
		} else if dur > 360 {
			clipDur = 240.0 // 4 phút nếu truyện dài
		}
		part := 1
		for start := 0.0; start < dur; start += clipDur {
			if part > 3 {
				break
			}
			length := clipDur
			if start+length > dur {
				length = dur - start
			}
			if length < 15 {
				break
			}
			clipName := fmt.Sprintf("studio-%s-clip%d.mp4", id, part)
			clipPath := filepath.Join(s.outDir, clipName)
			// Trích xuất clip 9:16 vertical cho TikTok/Reels/Shorts (-c copy nhanh & giữ nguyên chất lượng)
			args := []string{
				"-ss", fmt.Sprintf("%.2f", start),
				"-i", final,
				"-t", fmt.Sprintf("%.2f", length),
				"-c", "copy",
				clipPath,
			}
			if err := ffmpegRun(ctx, args...); err == nil {
				aid := s.addAsset(id, 100+part, "short_clip", fmt.Sprintf("Clip %d (%.0fs, 9:16 TikTok/Reels)", part, length))
				s.setAsset(aid, StatusDone, clipPath)
				s.appendLog(id, fmt.Sprintf("✓ Đã xuất Clip %d: %s (%.0fs, 9:16)", part, clipName, length))
			} else {
				s.appendLog(id, fmt.Sprintf("Trích xuất Clip %d: %v (bỏ qua)", part, err))
			}
			part++
		}
	}

	s.setStatus(id, StatusDone, 100)
	s.appendLog(id, fmt.Sprintf("Xong: %s (%.0fs, %d cảnh)", filepath.Base(final), dur, len(scenes)))
	s.fireOnDone(id)
}

func firstLine(title, text string) string {
	if title != "" {
		return title
	}
	if i := strings.Index(text, "\n"); i > 0 {
		return text[:i]
	}
	if r := []rune(text); len(r) > 60 {
		return string(r[:60]) + "…"
	}
	return text
}
