//go:build parked

package studio

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/ninhlee99/ai-creator-os/internal/engines"
	"path/filepath"
	"strings"
)

func (s *Studio) assembleFilmFinal(ctx context.Context, id string, p FilmParams, work string,
	rendered []renderedShot, subTexts []string, subDurs []float64, renderMode string) (string, error) {

	var clips []string
	for _, r := range rendered {
		clips = append(clips, r.MP4)
	}
	var cur string
	if renderMode == RenderModeCinematic {
		s.appendLog(id, fmt.Sprintf("Dựng điện ảnh: chuyển cảnh mượt %d shot…", len(clips)))
		var cc []CinematicClip
		for _, r := range rendered {
			cc = append(cc, CinematicClip{Path: r.MP4, CameraMove: r.Shot.CameraMove})
		}
		xfade := filepath.Join(work, "film_xfade.mp4")
		if err := AssembleCinematic(ctx, cc, xfade); err != nil {
			return "", err
		}
		cur = xfade
		// Letterbox 2.35:1 — viền điện ảnh, chỉ ở bản phim cuối (trailer
		// cắt từ shot gốc nên không dính viền).
		lb := filepath.Join(work, "film_letterbox.mp4")
		if err := ApplyLetterbox(ctx, cur, lb); err != nil {
			s.appendLog(id, "⚠ Letterbox lỗi ("+err.Error()+") — giữ bản không viền")
		} else {
			cur = lb
		}
		// Phụ đề theo đúng timeline xfade (shot sau bắt đầu sớm hơn 0.7s).
		if cues := cinematicCues(subTexts, subDurs); len(cues) > 0 {
			s.appendLog(id, fmt.Sprintf("Mux phụ đề (%d câu)…", len(cues)))
			subbed := filepath.Join(work, "film_subs.mp4")
			if err := MuxSubtitles(ctx, cur, formatSRT(cues), subbed); err != nil {
				return "", fmt.Errorf("mux phụ đề: %w", err)
			}
			cur = subbed
		}
	} else {
		s.appendLog(id, fmt.Sprintf("Nối %d shot…", len(clips)))
		concat := filepath.Join(work, "film_concat.mp4")
		if err := ConcatClips(ctx, clips, p.Aspect, concat); err != nil {
			return "", err
		}
		cur = concat
		// Phụ đề (P1-1): SRT từ thoại + lời dẫn, khớp thời lượng từng shot.
		var st []string
		var sd []float64
		for i, t := range subTexts {
			if strings.TrimSpace(t) != "" && i < len(subDurs) {
				st = append(st, t)
				sd = append(sd, subDurs[i])
			}
		}
		if len(st) > 0 {
			s.appendLog(id, fmt.Sprintf("Mux phụ đề (%d câu)…", len(st)))
			subbed := filepath.Join(work, "film_subs.mp4")
			if err := MuxSubtitles(ctx, cur, engines.BuildSRT(st, sd), subbed); err != nil {
				return "", fmt.Errorf("mux phụ đề: %w", err)
			}
			cur = subbed
		}
	}
	// Nhạc bed (P1-2): duck −8dB khi có voice + loudnorm −14 LUFS.
	if strings.TrimSpace(p.MusicPath) != "" {
		s.appendLog(id, "Mix nhạc nền…")
		mixed := filepath.Join(work, "film_music.mp4")
		if err := MixMusicBed(ctx, cur, p.MusicPath, mixed); err != nil {
			s.appendLog(id, "⚠ Mix nhạc lỗi ("+err.Error()+") — giữ bản không nhạc")
		} else {
			cur = mixed
		}
	}
	// Upscale cuối (P1-5): lanczos — KHÔNG phải 4K native, UI ghi rõ.
	final := filepath.Join(s.outDir, "studio-"+id+".mp4")
	if p.UpscaleFinal {
		w, h := AspectDims(p.Aspect)
		s.appendLog(id, "Upscale lanczos (không phải 4K native) — bước này lâu…")
		if err := upscaleVideoToDimensions(ctx, cur, final, w*2, h*2); err != nil {
			s.appendLog(id, "⚠ Upscale lỗi ("+err.Error()+") — giữ bản gốc")
			if err := copyFile(cur, final); err != nil {
				return "", err
			}
		}
	} else if err := copyFile(cur, final); err != nil {
		return "", err
	}
	return final, nil
}

// buildTrailers cắt tối đa 2 trailer dọc 9:16 từ các shot trailer_worthy
// (Ninh 2026-10-02): center-crop từ phim 16:9, không quay lại. Không có shot
// nào được đánh dấu → bỏ qua, không lỗi.
func (s *Studio) buildTrailers(ctx context.Context, id string, p FilmParams, work string, rendered []renderedShot) {
	var marked []renderedShot
	for _, r := range rendered {
		if r.Shot.TrailerWorthy {
			marked = append(marked, r)
		}
	}
	groups := selectTrailerGroups(marked)
	if len(groups) == 0 {
		s.appendLog(id, "Không có shot trailer_worthy — bỏ qua cắt trailer")
		return
	}
	for gi, g := range groups {
		var vertical []string
		var total float64
		ok := true
		for _, m := range g {
			dst := filepath.Join(work, fmt.Sprintf("trailer%d_%04d.mp4", gi, m.Shot.Seq))
			if err := CropCenterVertical(ctx, m.MP4, dst); err != nil {
				s.appendLog(id, fmt.Sprintf("Trailer %d: crop shot %d lỗi (%v) — bỏ trailer này", gi+1, m.Shot.Seq+1, err))
				ok = false
				break
			}
			vertical = append(vertical, dst)
			total += m.Dur
		}
		if !ok || len(vertical) == 0 {
			continue
		}
		// Clips đã 1080x1920 sau crop → ConcatClips "9:16" nối thẳng,
		// không scale thừa.
		raw := filepath.Join(work, fmt.Sprintf("trailer%d_raw.mp4", gi))
		if err := ConcatClips(ctx, vertical, "9:16", raw); err != nil {
			s.appendLog(id, fmt.Sprintf("Trailer %d: nối lỗi (%v)", gi+1, err))
			continue
		}
		cur := raw
		if strings.TrimSpace(p.MusicPath) != "" {
			mixed := filepath.Join(work, fmt.Sprintf("trailer%d.mp4", gi))
			if err := MixMusicBed(ctx, cur, p.MusicPath, mixed); err != nil {
				s.appendLog(id, fmt.Sprintf("Trailer %d: mix nhạc lỗi — giữ bản không nhạc", gi+1))
			} else {
				cur = mixed
			}
		}
		final := filepath.Join(s.outDir, fmt.Sprintf("studio-%s-trailer%d.mp4", id, gi+1))
		if err := copyFile(cur, final); err != nil {
			s.appendLog(id, fmt.Sprintf("Trailer %d: lưu lỗi (%v)", gi+1, err))
			continue
		}
		aid := s.addAsset(id, trailerIdxBase+gi, "trailer",
			fmt.Sprintf("Trailer %d — %d shot (%.0fs), crop dọc 9:16 từ phim", gi+1, len(g), total))
		s.setAssetMethod(aid, "trailer")
		s.setAsset(aid, StatusDone, final)
		s.appendLog(id, fmt.Sprintf("Trailer %d xong (%.0fs, 9:16)", gi+1, total))
	}
}

// TrailerShotSeqs returns the render-shot sequence numbers the director
// marked trailer_worthy, recomputed deterministically from the cached
// script.json — the storyboard UI uses it for the trailer badge.
func (s *Studio) TrailerShotSeqs(id string) []int {
	script, err := s.loadCachedScript(filepath.Join(s.workRoot, id))
	if err != nil {
		return nil
	}
	var out []int
	for _, sh := range expandScriptShots(script) {
		if sh.TrailerWorthy {
			out = append(out, sh.Seq)
		}
	}
	return out
}

// RerenderShot quay lại đúng một shot rồi dựng lại phim (storyboard UI /
// P1-3). Chạy đồng bộ — handler web dùng QueueRerenderShot để chạy nền.
func (s *Studio) RerenderShot(id string, seq int) error {
	if err := s.validateRerender(id, seq); err != nil {
		return err
	}
	j, _ := s.GetJob(id)
	var p FilmParams
	if err := json.Unmarshal([]byte(j.Params), &p); err != nil {
		return fmt.Errorf("đọc tham số job: %w", err)
	}
	work := filepath.Join(s.workRoot, id)
	script, err := s.loadCachedScript(work)
	if err != nil {
		return fmt.Errorf("không đọc được kịch bản: %w", err)
	}
	shots := expandScriptShots(script)
	sh := shots[seq]
	scenes := sceneMap(script)
	var charRefs []ImageRef
	for i := range script.Characters {
		if pp := s.resumeAssetPath(id, portraitIdxBase+i, "portrait"); pp != "" {
			charRefs = append(charRefs, ImageRef{Path: pp})
		}
	}
	prevMp4 := ""
	if seq > 0 {
		prevMp4 = s.resumeAssetPath(id, seq-1, "shot")
	}
	budget := &veoBudget{rate: veoRateUSD()}
	if s.BudgetUSD != nil {
		budget.cap = s.BudgetUSD()
	}
	ctx := context.Background()
	s.appendLog(id, fmt.Sprintf("Quay lại shot %d…", seq+1))
	s.setStatus(id, StatusRunning, 50)
	renderMode := s.resolveRenderMode(p)
	if _, err := s.renderFilmShot(ctx, id, p, work, scenes[sh.SceneIdx], characterLocks(script), charRefs, sh, prevMp4, budget, renderMode); err != nil {
		s.setStatus(id, StatusFailed, 100)
		return err
	}
	return s.ReassembleFilm(id)
}

// validateRerender checks everything cheap before a shot re-render starts
// (job exists, is a film, idle, seq valid, script cached).
func (s *Studio) validateRerender(id string, seq int) error {
	j, ok := s.GetJob(id)
	if !ok {
		return fmt.Errorf("job không tồn tại")
	}
	if j.Kind != KindFilm {
		return fmt.Errorf("chỉ job phim mới quay lại shot được")
	}
	s.mu.Lock()
	_, already := s.running[id]
	s.mu.Unlock()
	if already {
		return fmt.Errorf("job đang chạy — đợi xong rồi quay lại")
	}
	work := filepath.Join(s.workRoot, id)
	script, err := s.loadCachedScript(work)
	if err != nil {
		return fmt.Errorf("không đọc được kịch bản: %w", err)
	}
	if seq < 0 || seq >= len(expandScriptShots(script)) {
		return fmt.Errorf("shot %d không tồn tại", seq+1)
	}
	return nil
}

// QueueRerenderShot validates synchronously, then re-renders the shot and
// re-assembles the film in the background (a Veo shot takes minutes — the
// HTTP handler must not block).
func (s *Studio) QueueRerenderShot(id string, seq int) error {
	if err := s.validateRerender(id, seq); err != nil {
		return err
	}
	go func() {
		if err := s.RerenderShot(id, seq); err != nil {
			s.appendLog(id, "Quay lại shot lỗi: "+err.Error())
			s.setStatus(id, StatusFailed, 100)
		}
	}()
	return nil
}

// ReassembleFilm dựng lại phim từ các shot đã xong (sau "Quay lại shot này").
func (s *Studio) ReassembleFilm(id string) error {
	j, ok := s.GetJob(id)
	if !ok {
		return fmt.Errorf("job không tồn tại")
	}
	var p FilmParams
	if err := json.Unmarshal([]byte(j.Params), &p); err != nil {
		return fmt.Errorf("đọc tham số job: %w", err)
	}
	work := filepath.Join(s.workRoot, id)
	script, err := s.loadCachedScript(work)
	if err != nil {
		return err
	}
	shots := expandScriptShots(script)
	ctx := context.Background()
	var rendered []renderedShot
	var subTexts []string
	var subDurs []float64
	for _, sh := range shots {
		mp4 := s.resumeAssetPath(id, sh.Seq, "shot")
		if mp4 == "" {
			return fmt.Errorf("shot %d chưa dựng xong — không dựng lại được", sh.Seq+1)
		}
		d := ProbeDuration(ctx, mp4)
		if d <= 0 {
			d = float64(sh.Seconds)
		}
		rendered = append(rendered, renderedShot{Shot: sh, MP4: mp4, Dur: d})
		if t := shotSpokenText(sh); strings.TrimSpace(t) != "" {
			subTexts = append(subTexts, t)
			subDurs = append(subDurs, d)
		}
	}
	s.appendLog(id, "Dựng lại phim từ các shot…")
	final, err := s.assembleFilmFinal(ctx, id, p, work, rendered, subTexts, subDurs, s.resolveRenderMode(p))
	if err != nil {
		s.setStatus(id, StatusFailed, 100)
		return err
	}
	s.setOutput(id, filepath.Base(final))
	s.setStatus(id, StatusDone, 100)
	s.appendLog(id, "Dựng lại xong: "+filepath.Base(final))
	s.buildTrailers(ctx, id, p, work, rendered)
	s.fireOnDone(id)
	return nil
}

// ---------------------------------------------------------------------------
// capability probe Veo (park cùng pipeline phim)
// ---------------------------------------------------------------------------

// VideoCapOK báo key có quay được video không (theo lần kiểm tra gần nhất).
// videoCapStatus đọc trạng thái video_gen trực tiếp từ DB (GetCapabilities
// ở bản thường đã bỏ hàng video — parked code không dùng nó nữa).
