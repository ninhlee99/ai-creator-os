//go:build parked

package studio

import (
	"context"
	"fmt"
	"github.com/ninhlee99/ai-creator-os/internal/engines"
	"github.com/ninhlee99/ai-creator-os/internal/engines/tts"
	"os"
	"path/filepath"
	"strings"
)

func (s *Studio) renderFilmShot(ctx context.Context, id string, p FilmParams, work string,
	sc FilmScenePro, charLocks string, charRefs []ImageRef,
	sh RenderShot, prevMp4 string, budget *veoBudget, renderMode string) (*renderedShot, error) {

	orient := orientationWord(p.Aspect)
	aid := s.addAsset(id, sh.Seq, "shot", sh.ImagePrompt)
	mp4 := filepath.Join(work, fmt.Sprintf("shot%04d.mp4", sh.Seq))
	s.setAsset(aid, StatusRunning, "")

	keyPath := filepath.Join(work, fmt.Sprintf("shot%04d.png", sh.Seq))
	// Continuity bible: khối bất di bất dịch của cảnh — kế thừa NGUYÊN VĂN
	// vào mọi prompt, không diễn đạt lại (continuity lock từng khung hình).
	contBlock := ""
	if strings.TrimSpace(sc.Continuity) != "" {
		contBlock = "\nCONTINUITY BIBLE — copy verbatim into the frame, do not paraphrase or alter:\n" +
			strings.TrimSpace(sc.Continuity)
	}
	keyPrompt := sh.ImagePrompt + charLocks + "\n" + sc.SceneLockBlock() + contBlock +
		" Cinematic photorealistic, " + orient + ", no text, no watermark."

	if renderMode == RenderModeVeo {
		if rs, ok, ferr := s.tryVeoShot(ctx, id, p, work, sc, charLocks, charRefs,
			sh, prevMp4, budget, aid, mp4, keyPath, keyPrompt, orient, contBlock); ferr != nil {
			return nil, ferr
		} else if ok {
			return rs, nil
		}
		// Veo lỗi → rơi về điện ảnh từ ảnh (log đã ghi trong tryVeoShot).
	}
	return s.renderCinematicShot(ctx, id, p, work, sc, charLocks, charRefs,
		sh, aid, mp4, keyPath, keyPrompt)
}

// tryVeoShot attempts one ≤8s Veo render with firstFrame chaining.
// (rs, true, nil) = quay được; (nil, false, nil) = Veo lỗi → caller rơi về
// điện ảnh từ ảnh; (nil, false, err) = lỗi nghiêm trọng (vượt trần chi phí).
func (s *Studio) tryVeoShot(ctx context.Context, id string, p FilmParams, work string,
	sc FilmScenePro, charLocks string, charRefs []ImageRef,
	sh RenderShot, prevMp4 string, budget *veoBudget,
	aid int64, mp4, keyPath, keyPrompt, orient, contBlock string) (*renderedShot, bool, error) {

	// FirstFrame: shot đầu dùng keyframe của chính nó; các shot sau dùng
	// frame cuối của shot trước (không cắt được frame → vẽ keyframe mới).
	firstFrame := ""
	if sh.Seq == 0 || prevMp4 == "" {
		if kerr := s.mg.GenerateImage(ctx, keyPrompt, charRefs, keyPath); kerr != nil {
			s.setAsset(aid, StatusFailed, "")
			return nil, false, fmt.Errorf("keyframe: %w", kerr)
		}
		firstFrame = keyPath
	} else if err := extractLastFrame(ctx, prevMp4, keyPath); err != nil {
		if kerr := s.mg.GenerateImage(ctx, keyPrompt, charRefs, keyPath); kerr != nil {
			s.setAsset(aid, StatusFailed, "")
			return nil, false, fmt.Errorf("keyframe: %w", kerr)
		}
		firstFrame = keyPath
	} else {
		firstFrame = keyPath
	}

	// QC chống drift (mặc định no-op trung thực — xem qc.go).
	if s.QC != nil {
		_ = s.QC.CheckShot(ctx, firstPortraitPath(charRefs), keyPath, "")
	}

	// Trần chi phí: kiểm tra TRƯỚC mỗi lần gọi Veo (P0-5).
	billed := float64(min(sh.Seconds, veoMaxSeconds))
	if err := budget.check(billed); err != nil {
		s.setAsset(aid, StatusFailed, "")
		return nil, false, fmt.Errorf("%w: %v", errBudgetExceeded, err)
	}
	vprompt := fmt.Sprintf("%s. Camera: %s, %s, %s. %s cinematic film, natural motion, no text.",
		sh.ImagePrompt, sh.ShotSize, sh.CameraMove, sh.LensLight, orient) +
		charLocks + "\n" + sc.SceneLockBlock() + contBlock + performanceBlock(sh)
	s.appendLog(id, fmt.Sprintf("Quay shot %d (%ds)…", sh.Seq+1, sh.Seconds))
	if err := s.mg.GenerateVideo(ctx, vprompt, firstFrame, sh.Seconds, p.Aspect, mp4); err != nil {
		s.appendLog(id, fmt.Sprintf("Shot %d: Veo lỗi (%v) — dùng điện ảnh từ ảnh", sh.Seq+1, err))
		return nil, false, nil
	}
	budget.spentSecs += billed
	s.appendLog(id, fmt.Sprintf("Shot %d: Veo ✓ (~$%.2f ước tính)", sh.Seq+1, billed*budget.rate))
	// Veo thành công → key quay được video (capability học từ lần chạy thật).
	_ = s.SetCapability(CapVideoGen, CapOK, fmt.Sprintf("shot %d quay bằng Veo thành công", sh.Seq+1))
	dur := ProbeDuration(ctx, mp4)
	if dur <= 0 {
		dur = float64(sh.Seconds)
	}
	s.setAssetMethod(aid, "veo")
	s.setAsset(aid, StatusDone, mp4)
	return &renderedShot{Shot: sh, MP4: mp4, Dur: dur}, true, nil
}

// renderCinematicShot renders one shot in the primary "cinematic stills"
// mode (Film Wave 3): keyframe (bố cục chừa biên cho chuyển động —
// prompts/film_keyframe.txt) → chuyển động điện ảnh theo đúng camera_move
// của đạo diễn → voice/silence audio. Không tốn Veo.
func (s *Studio) renderCinematicShot(ctx context.Context, id string, p FilmParams, work string,
	sc FilmScenePro, charLocks string, charRefs []ImageRef,
	sh RenderShot, aid int64, mp4, keyPath, keyPrompt string) (*renderedShot, error) {

	cinePrompt := keyPrompt + keyframeCompBlock()
	move := strings.ToLower(strings.TrimSpace(sh.CameraMove))
	if move == "" {
		move = "drift"
	}
	s.appendLog(id, fmt.Sprintf("Shot %d: dựng điện ảnh từ ảnh (%s)…", sh.Seq+1, move))
	if kerr := s.mg.GenerateImage(ctx, cinePrompt, charRefs, keyPath); kerr != nil {
		s.setAsset(aid, StatusFailed, "")
		return nil, fmt.Errorf("keyframe: %w", kerr)
	}

	// QC chống drift (mặc định no-op trung thực — xem qc.go).
	if s.QC != nil {
		_ = s.QC.CheckShot(ctx, firstPortraitPath(charRefs), keyPath, "")
	}

	// Giọng đọc của shot (TTS từng đoạn ≤800 ký tự — P0-6); lỗi → bản câm.
	wavPath := s.synthShotVoice(ctx, id, sh, work)
	dur := float64(sh.Seconds)
	if wavPath != "" {
		if ws, err := engines.WavSeconds(wavPath); err == nil && ws > dur {
			dur = ws
		}
	}
	tmpV := filepath.Join(work, fmt.Sprintf("shot%04d.cine.mp4", sh.Seq))
	if err := RenderCinematicShot(ctx, CinematicShot{
		ImagePath: keyPath, Seconds: dur,
		CameraMove: sh.CameraMove, Mood: sc.Atmosphere,
	}, tmpV); err != nil {
		s.setAsset(aid, StatusFailed, "")
		return nil, fmt.Errorf("dựng điện ảnh: %w", err)
	}
	defer os.Remove(tmpV)
	if err := AddShotAudio(ctx, tmpV, wavPath, dur, mp4); err != nil {
		s.setAsset(aid, StatusFailed, "")
		return nil, fmt.Errorf("gắn giọng đọc: %w", err)
	}
	d := ProbeDuration(ctx, mp4)
	if d <= 0 {
		d = dur
	}
	s.setAssetMethod(aid, "cinematic")
	s.setAsset(aid, StatusDone, mp4)
	return &renderedShot{Shot: sh, MP4: mp4, Dur: d}, nil
}

// synthShotVoice synthesizes a shot's dialogue + narration in ≤800-char
// chunks (P0-6) and joins them into one WAV. Returns "" when there is
// nothing to say or synthesis fails — fail-soft: silent shot, logged.
func (s *Studio) synthShotVoice(ctx context.Context, id string, sh RenderShot, work string) string {
	speech := shotSpokenText(sh)
	if s.narrator == nil || strings.TrimSpace(speech) == "" {
		return ""
	}
	var wavs [][]byte
	for _, ch := range splitTextChunks(speech, maxTTSChunkChars) {
		w, err := s.narrator(ctx, ch)
		if err != nil || len(w) == 0 {
			s.appendLog(id, fmt.Sprintf("Shot %d: TTS lỗi (%v) — dựng bản câm", sh.Seq+1, err))
			return ""
		}
		wavs = append(wavs, w)
	}
	joined, jerr := tts.ConcatWavs(wavs)
	if jerr != nil {
		s.appendLog(id, fmt.Sprintf("Shot %d: nối WAV lỗi (%v) — dựng bản câm", sh.Seq+1, jerr))
		return ""
	}
	wavPath := filepath.Join(work, fmt.Sprintf("shot%04d.wav", sh.Seq))
	if werr := os.WriteFile(wavPath, joined, 0o644); werr != nil {
		s.appendLog(id, fmt.Sprintf("Shot %d: không ghi được WAV: %v", sh.Seq+1, werr))
		return ""
	}
	return wavPath
}

// assembleFilmFinal nối các shot → mux phụ đề → mix nhạc bed → upscale
// (nếu chọn) → file cuối trong outDir. Dùng chung cho runFilm và dựng lại
// sau "Quay lại shot này".
//
// Film Wave 3: chế độ "cinematic" dựng bằng xfade (chuyển cảnh mượt) +
// letterbox 2.35:1 + phụ đề theo đúng timeline xfade; chế độ "veo" giữ
// đường nối cứng cũ.
